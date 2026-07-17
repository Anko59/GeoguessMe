package integration_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFullGameFlow(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Flow Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := acceptChallenge(t, bob.access, photoID)
	require.True(t, strings.HasPrefix(acc.MediaURL, "/api/v1/challenges/"), "media must be same-origin, got %q", acc.MediaURL)

	// Guessing is rejected while the viewing window is open.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
		map[string]float64{"lat": 51.505, "long": -0.09}, bob.access, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	waitUntilViewExpires(t, acc.ViewExpiresAt)
	status := guess(t, bob.access, photoID, 51.505, -0.09)
	require.Equal(t, http.StatusCreated, status)

	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var entries []struct {
		Username   string `json:"username"`
		GuessCount int    `json:"guess_count"`
		Score      int    `json:"score"`
	}
	require.NoError(t, jsonUnmarshal(data, &entries))
	var bobEntry *struct {
		Username   string `json:"username"`
		GuessCount int    `json:"guess_count"`
		Score      int    `json:"score"`
	}
	for i := range entries {
		if strings.HasPrefix(entries[i].Username, "bob") {
			bobEntry = &entries[i]
		}
	}
	require.NotNil(t, bobEntry, "bob must appear in the leaderboard")
	require.Equal(t, 1, bobEntry.GuessCount)
	require.Greater(t, bobEntry.Score, 0)
}

func TestGuessRejectedDuringViewingWindow(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Window Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := acceptChallenge(t, bob.access, photoID)
	require.True(t, time.Now().Before(acc.ViewExpiresAt.Add(2*time.Second)))
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
		map[string]float64{"lat": 0, "long": 0}, bob.access, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestReAcceptDoesNotExtendWindow(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Reaccept Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	first := acceptChallenge(t, bob.access, photoID)
	waitUntilViewExpires(t, first.ViewExpiresAt)
	second := acceptChallenge(t, bob.access, photoID)
	// Within microsecond precision (DB storage rounds nanosecond time).
	require.True(t, first.ViewExpiresAt.UTC().Sub(second.ViewExpiresAt.UTC()).Abs() < time.Microsecond,
		"re-accepting must not extend or reset the viewing window")
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestResultVisibilityAuthorization(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	carol := signup(t, uniqueU("carol"), uniqueU("carol")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Results Group")
	joinGroup(t, bob.access, code)
	joinGroup(t, carol.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	acc := acceptChallenge(t, bob.access, photoID)
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	status := guess(t, bob.access, photoID, 10, 10)
	require.Equal(t, http.StatusCreated, status)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, carol.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestConcurrentDuplicateGuess(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Dup Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := acceptChallenge(t, bob.access, photoID)
	waitUntilViewExpires(t, acc.ViewExpiresAt)

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	start := make(chan struct{})
	results := make([]int, workers)
	body := map[string]float64{"lat": 1, "long": 1}
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			resp, _ := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess", body, bob.access, nil)
			results[i] = resp.StatusCode
		}()
	}
	close(start)
	wg.Wait()

	created, dups, other := 0, 0, 0
	for _, c := range results {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			dups++
		default:
			other++
		}
	}
	require.Equalf(t, 1, created, "exactly one guess must be created (dups=%d, other=%d)", dups, other)
	require.Equal(t, workers-1, dups)
	require.Equal(t, 0, other)

	resp, data := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var res struct {
		Guesses []struct {
			ID string `json:"id"`
		} `json:"guesses"`
	}
	require.NoError(t, jsonUnmarshal(data, &res))
	require.Len(t, res.Guesses, 1, "concurrent guesses must collapse to a single row")
}

func TestMediaIsRemovedAfterViewWindow(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Media Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := acceptChallenge(t, bob.access, photoID)
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func uniqueU(name string) string { return unique(name) }
