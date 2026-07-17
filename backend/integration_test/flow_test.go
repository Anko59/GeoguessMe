package integration_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFullGameFlow(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")

	groupID, code := createGroup(t, alice.access, "Flow Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bob.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "accept: %s", data)
	var accept struct {
		MediaURL string `json:"media_url"`
	}
	require.NoError(t, jsonUnmarshal(data, &accept))
	require.True(t, strings.HasPrefix(accept.MediaURL, "/api/v1/challenges/"), "media must be same-origin, got %q", accept.MediaURL)

	waitUntilViewCloses(t, bob.access, photoID)
	guess(t, bob.access, photoID, 51.505, -0.09)

	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(data), "bob")
}

func TestGuessRejectedDuringViewingWindow(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Window Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	resp, _ := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
		map[string]float64{"lat": 0, "long": 0}, bob.access, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestReAcceptDoesNotExtendWindow(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Reaccept Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	_, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bob.access, nil)
	var first struct {
		ViewExpiresAt string `json:"view_expires_at"`
	}
	require.NoError(t, jsonUnmarshal(data, &first))

	_, data = doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bob.access, nil)
	var second struct {
		ViewExpiresAt string `json:"view_expires_at"`
	}
	require.NoError(t, jsonUnmarshal(data, &second))
	require.Equal(t, first.ViewExpiresAt, second.ViewExpiresAt, "re-accepting must not extend the viewing window")
}

func TestResultVisibilityAuthorization(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("carol"), unique("carol")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Results Group")
	joinGroup(t, bob.access, code)
	joinGroup(t, carol.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	// The uploader may view results immediately.
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A member who has not guessed cannot view results before expiry.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// After guessing, results become available to that member.
	waitUntilViewCloses(t, bob.access, photoID)
	guess(t, bob.access, photoID, 10, 10)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Carol still has not guessed and cannot see results.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, carol.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestConcurrentDuplicateGuess(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Dup Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)
	waitUntilViewCloses(t, bob.access, photoID)

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			resp, _ := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
				map[string]float64{"lat": 1, "long": 1}, bob.access, nil)
			require.Contains(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode)
		}()
	}
	wg.Wait()

	resp, data := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var results struct {
		Guesses []struct{} `json:"guesses"`
	}
	require.NoError(t, jsonUnmarshal(data, &results))
	require.Len(t, results.Guesses, 1, "concurrent guesses must collapse to one")
}

func TestMediaIsRemovedAfterViewWindow(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Media Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	_, _ = doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bob.access, nil)
	// During the window the media endpoint serves the image.
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	waitUntilViewCloses(t, bob.access, photoID)
	// After the window, the same authenticated path is forbidden.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
