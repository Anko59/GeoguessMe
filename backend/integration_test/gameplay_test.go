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

// TestGuessRejectedAfterGuessWindowExpiry pins the server-authoritative guess
// deadline end-to-end: accept and delivery publish guess_expires_at (view end
// + GUESS_WINDOW), and a guess submitted after it is refused with 410
// guess_time_expired without creating a guess row (the challenge counts as 0
// points for that member). The recorded deadline is pushed into the past
// directly so the test does not wait for the configured GUESS_WINDOW.
func TestGuessRejectedAfterGuessWindowExpiry(t *testing.T) {
	alice := signup(t, unique("gwalice"), unique("gwalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("gwbob"), unique("gwbob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Guess Window Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
	require.True(t, strings.HasPrefix(acc.MediaURL, "/api/v1/challenges/"), "media must be same-origin, got %q", acc.MediaURL)
	// The guess deadline is the view end plus the default GUESS_WINDOW (2m),
	// published by both the accept and the delivery responses.
	require.False(t, acc.GuessExpiresAt.IsZero(), "accept/delivery must publish guess_expires_at")
	require.WithinDuration(t, acc.ViewExpiresAt.Add(2*time.Minute), acc.GuessExpiresAt, time.Second,
		"guess deadline must be view end + GUESS_WINDOW")

	waitUntilViewExpires(t, acc.ViewExpiresAt)

	// A guess inside the window is accepted...
	require.Equal(t, http.StatusCreated, guess(t, bob.access, photoID, 51.505, -0.09))

	// ...and after the (moved) deadline the same challenge refuses a second
	// guess outright: idempotency reads the existing row, so the refusal must
	// be tested with a fresh challenge that never got a guess.
	photoID2 := uploadPhoto(t, alice.access, groupID)
	acc2 := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID2))
	waitUntilViewExpires(t, acc2.ViewExpiresAt)
	db := testDB(t)
	_, err := db.Exec(t.Context(),
		`UPDATE challenge_views SET guess_expires_at = NOW() - interval '1 second' WHERE photo_id = $1 AND user_id = $2`,
		photoID2, bob.userID)
	require.NoError(t, err)

	resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID2+"/guess",
		map[string]float64{"lat": 51.505, "long": -0.09}, bob.access, nil)
	require.Equal(t, http.StatusGone, resp.StatusCode)
	require.Contains(t, string(data), "guess_time_expired")
	var count int
	require.NoError(t, db.QueryRow(t.Context(),
		`SELECT count(*) FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID2, bob.userID).Scan(&count))
	require.Zero(t, count, "a refused late guess must not create a guess row")
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

// submitPartyGuess submits a guess and returns its recorded score plus the
// party_doubled flag from the wire response.
func submitPartyGuess(t *testing.T, bearer, photoID string) (score int, partyDoubled bool) {
	t.Helper()
	accepted := deliverChallengeMedia(t, bearer, acceptChallenge(t, bearer, photoID))
	waitUntilViewExpires(t, accepted.ViewExpiresAt)
	resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
		map[string]float64{"lat": 51.5055, "long": -0.0905}, bearer, nil)
	require.Containsf(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode, "guess %d: %s", resp.StatusCode, data)
	var body struct {
		Score        int  `json:"score"`
		PartyDoubled bool `json:"party_doubled"`
	}
	require.NoError(t, jsonUnmarshal(data, &body))
	return body.Score, body.PartyDoubled
}

// TestPartyTimeDoublePointsForChallengePosters pins the Party Time feature
// end to end against a live database: any member may start a party, a second
// start while one is active is refused, the persisted system message
// announces the starter by name, and — the core rule — a member who posted a
// challenge during the active window scores exactly double on their guess,
// while a member who posted nothing scores the base value. The recharge
// branch (48h measured from the previous end) is pinned by repository unit
// tests; no test endpoint can fast-forward the server clock.
func TestPartyTimeDoublePointsForChallengePosters(t *testing.T) {
	alice := signup(t, unique("ptalice"), unique("ptalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("ptbob"), unique("ptbob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("ptcarol"), unique("ptcarol")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Party Group")
	joinGroup(t, bob.access, inviteToken)
	joinGroup(t, carol.access, inviteToken)

	// A fresh group is immediately startable: no active flag, no cooldown.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/party?group_id="+groupID, nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var initial map[string]any
	require.NoError(t, jsonUnmarshal(data, &initial))
	require.Equal(t, false, initial["active"])
	require.NotContains(t, initial, "next_available_at")

	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/party", map[string]string{"group_id": groupID}, alice.access, nil)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "start %d: %s", resp.StatusCode, data)
	var started struct {
		Active    bool   `json:"active"`
		EndsAt    string `json:"ends_at"`
		ServerNow string `json:"server_time"`
	}
	require.NoError(t, jsonUnmarshal(data, &started))
	require.True(t, started.Active)
	endsAt, err := time.Parse(time.RFC3339Nano, started.EndsAt)
	require.NoError(t, err)
	serverStart, err := time.Parse(time.RFC3339Nano, started.ServerNow)
	require.NoError(t, err)
	require.WithinDurationf(t, serverStart.Add(57*time.Minute), endsAt, 3*time.Minute+30*time.Second,
		"party must run roughly PARTY_TIME_DURATION (ends_at %s)", started.EndsAt)

	// The status endpoint reports the same window to other members.
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/party?group_id="+groupID, nil, carol.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(data), `"active":true`)

	// A second start while one is running is refused with the dedicated code.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/party", map[string]string{"group_id": groupID}, bob.access, nil)
	require.Equalf(t, http.StatusConflict, resp.StatusCode, "second start %d: %s", resp.StatusCode, data)
	require.Contains(t, string(data), "party_active")

	// The announcement lands as a persisted system chat message naming the starter.
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&limit=50", nil, carol.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Items []struct {
			Kind    string `json:"kind"`
			Content string `json:"content"`
		} `json:"items"`
	}
	require.NoError(t, jsonUnmarshal(data, &page))
	foundAnnouncement := false
	for _, item := range page.Items {
		if item.Kind == "system" && strings.Contains(item.Content, "started Party Time!") {
			foundAnnouncement = true
		}
	}
	require.True(t, foundAnnouncement, "the party announcement system message must be in history")

	// Both members post a challenge into the group during the window.
	alicePhoto := uploadPhoto(t, alice.access, groupID)
	_ = uploadPhoto(t, bob.access, groupID)

	// carol never posts: her identical guess records the base score without
	// the party flag. bob posted during the window: his identical guess on
	// alice's challenge scores exactly double.
	baseScore, baseDoubled := submitPartyGuess(t, carol.access, alicePhoto)
	posterScore, posterDoubled := submitPartyGuess(t, bob.access, alicePhoto)
	require.False(t, baseDoubled, "a member who did not post must not be doubled")
	require.True(t, posterDoubled, "a member who posted during the window must be doubled")
	require.Greater(t, baseScore, 0, "the scenario needs a non-trivial base score")
	require.Equalf(t, 2*baseScore, posterScore, "doubled score %d must equal twice the base %d", posterScore, baseScore)
}
