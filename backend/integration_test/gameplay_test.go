package integration_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConcurrentMediaDeliveryConfirmation races many delivery confirmations
// against the same challenge view and pins the view-once guarantee: the first
// confirmation starts the window and every concurrent confirmation returns the
// same authoritative deadline without extending it.
func TestConcurrentMediaDeliveryConfirmation(t *testing.T) {
	alice := signup(t, unique("cdalice"), unique("cdalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("cdbob"), unique("cdbob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Concurrent Delivery Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)
	// A single accept records the view row; the delivery confirmations then race.
	acceptChallenge(t, bob.access, photoID)

	const workers = 12
	start := make(chan struct{})
	deadlines := make([]time.Time, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/media-delivered", nil, bob.access, nil)
			if resp.StatusCode != http.StatusOK {
				deadlines[i] = time.Time{} // sentinel for assertion below
				return
			}
			var body struct {
				ViewExpiresAt string `json:"view_expires_at"`
			}
			if err := jsonUnmarshal(data, &body); err != nil {
				return
			}
			deadline, err := time.Parse(time.RFC3339Nano, body.ViewExpiresAt)
			if err != nil {
				return
			}
			deadlines[i] = deadline
		}()
	}
	close(start)
	wg.Wait()

	first := deadlines[0]
	require.False(t, first.IsZero(), "first delivery confirmation must return a deadline")
	for i, deadline := range deadlines[1:] {
		require.Truef(t, deadline.Equal(first), "concurrent delivery %d extended the window: %v vs %v", i, deadline, first)
	}
}

// TestLeaderboardRankingDeterminism pins ranking determinism: reading the same
// group leaderboard twice must produce a byte-identical response, and the Elo
// ordering must follow guess quality (a closer guess ranks above a farther
// one) because Elo is recomputed from the period's challenges.
func TestLeaderboardRankingDeterminism(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("carol"), unique("carol")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Ranking Determinism Group")
	joinGroup(t, bob.access, code)
	joinGroup(t, carol.access, code)
	// uploadPhoto places the challenge at (51.505, -0.09); bob guesses exactly
	// there (perfect score), carol guesses far away (low score).
	photoID := uploadPhoto(t, alice.access, groupID)
	for _, bearer := range []string{bob.access, carol.access} {
		acc := deliverChallengeMedia(t, bearer, acceptChallenge(t, bearer, photoID))
		waitUntilViewExpires(t, acc.ViewExpiresAt)
	}
	guess(t, bob.access, photoID, 51.505, -0.09)
	guess(t, carol.access, photoID, 40.0, -3.0)

	url := "/api/v1/group/leaderboard?group_id=" + groupID + "&metric=elo"
	resp1, data1 := doJSON(t, http.MethodGet, url, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp1.StatusCode)
	resp2, data2 := doJSON(t, http.MethodGet, url, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	require.Equal(t, data1, data2, "leaderboard must be deterministic across reads")

	var entries []struct {
		Username   string `json:"username"`
		GuessCount int    `json:"guess_count"`
		Elo        int    `json:"elo"`
	}
	require.NoError(t, jsonUnmarshal(data1, &entries))
	bobElo, carolElo := -1, -1
	for _, entry := range entries {
		switch {
		case strings.HasPrefix(entry.Username, "bob"):
			require.Equal(t, 1, entry.GuessCount)
			require.Greater(t, entry.Elo, 0, "rated player must have a positive elo")
			bobElo = entry.Elo
		case strings.HasPrefix(entry.Username, "carol"):
			carolElo = entry.Elo
		}
	}
	require.Greaterf(t, bobElo, 0, "bob must be rated")
	require.Greaterf(t, carolElo, 0, "carol must be rated")
	require.Greaterf(t, bobElo, carolElo, "closer guess must rank above the farther one (bob=%d, carol=%d)", bobElo, carolElo)
}
