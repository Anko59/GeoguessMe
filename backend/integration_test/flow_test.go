package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"geoguessme/internal/models"
	feedrepo "geoguessme/internal/repository/feed"

	"github.com/stretchr/testify/require"
)

func TestFullGameFlow(t *testing.T) {
	alice := signup(t, uniqueU("alice"), uniqueU("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, uniqueU("bob"), uniqueU("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Flow Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
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

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
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
	// A player who never received the media in full can still fetch it after
	// the accept window: the viewing window starts at the first delivery, so a
	// slow connection gets the full viewing time.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "never-delivered media after accept window: %s", data)
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

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
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

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
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

	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func uniqueU(name string) string { return unique(name) }

func uploadPublicPhoto(t *testing.T, bearer string) string {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("photo", "public.png")
	require.NoError(t, err)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = file.Write(png)
	require.NoError(t, err)
	require.NoError(t, form.WriteField("lat", "48.8"))
	require.NoError(t, form.WriteField("long", "2.3"))
	require.NoError(t, form.WriteField("caption", "Find this public place"))
	require.NoError(t, form.Close())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/feed/challenges", &body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, 201, resp.StatusCode, "%s", data)
	var result struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	return result.ID
}

// This fixture exercises migration 026 against the real database and HTTP
// handlers, including viewer isolation, concurrent guesses and cascade cleanup.
func TestPublicFeedJourney(t *testing.T) {
	owner := signup(t, uniqueU("poster"), uniqueU("poster")+"@example.test", "StrongPassword123")
	viewer := signup(t, uniqueU("viewer"), uniqueU("viewer")+"@example.test", "StrongPassword123")
	other := signup(t, uniqueU("other"), uniqueU("other")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, owner.access, "Private circle")
	privateID := uploadPhoto(t, owner.access, groupID)
	publicID := uploadPublicPhoto(t, owner.access)
	path := "/api/v1/feed/challenges/" + publicID
	resp, _ := doJSON(t, "GET", "/api/v1/feed", nil, "", nil)
	require.Equal(t, 401, resp.StatusCode)
	resp, data := doJSON(t, "GET", "/api/v1/feed", nil, viewer.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NotContains(t, string(data), privateID)
	require.NotContains(t, string(data), "storage_key")
	require.NotContains(t, string(data), "actual_lat")
	var page models.PublicFeedPage
	require.NoError(t, json.Unmarshal(data, &page))
	require.Contains(t, string(data), publicID)
	resp, _ = doJSON(t, "GET", path+"/guess", nil, viewer.access, nil)
	require.Equal(t, 404, resp.StatusCode)
	resp, preview := doJSON(t, "GET", path+"/media", nil, viewer.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "private, no-store", resp.Header.Get("Cache-Control"))
	resp, original := doJSON(t, "GET", path+"/play", nil, viewer.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NotEqual(t, preview, original)
	resp, ownerMedia := doJSON(t, "GET", path+"/media", nil, owner.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, original, ownerMedia)
	resp, _ = doJSON(t, "POST", path+"/guess", map[string]float64{"lat": 48.8, "long": 2.3}, owner.access, nil)
	require.Equal(t, 403, resp.StatusCode)

	// Both simultaneous submissions must observe the same immutable winner.
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([][]byte, 2)
	statuses := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response, body := doJSON(t, "POST", path+"/guess", map[string]float64{"lat": 48.8 + float64(i), "long": 2.3}, viewer.access, nil)
			results[i], statuses[i] = body, response.StatusCode
		}()
	}
	close(start)
	wg.Wait()
	require.Equal(t, []int{200, 200}, statuses)
	require.JSONEq(t, string(results[0]), string(results[1]))
	resp, revealed := doJSON(t, "GET", path+"/media", nil, viewer.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, original, revealed)
	_, stillBlurred := doJSON(t, "GET", path+"/media", nil, other.access, nil)
	require.Equal(t, preview, stillBlurred)
	for range 2 {
		resp, _ = doJSON(t, "PUT", path+"/reaction", nil, viewer.access, nil)
		require.Equal(t, 204, resp.StatusCode)
	}
	_, data = doJSON(t, "GET", path, nil, viewer.access, nil)
	var post models.PublicChallenge
	require.NoError(t, json.Unmarshal(data, &post))
	require.True(t, post.Resolved)
	require.True(t, post.Reacted)
	require.Equal(t, 1, post.ReactionCount)
	_, data = doJSON(t, "GET", path, nil, other.access, nil)
	require.NoError(t, json.Unmarshal(data, &post))
	require.False(t, post.Resolved)
	require.False(t, post.Reacted)
	// Another player's shared parent lock must not serialize this attempt.
	// The deadline is a failure bound, not a synchronization delay.
	db := testDB(t)
	lock, err := db.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = lock.Rollback(t.Context()) }()
	_, err = lock.Exec(t.Context(), `SELECT id FROM public_challenges WHERE id=$1 FOR KEY SHARE`, publicID)
	require.NoError(t, err)
	guessCtx, cancelGuess := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelGuess()
	_, err = feedrepo.NewRepository(db).Guess(guessCtx, publicID, other.userID, 0, 0)
	require.NoError(t, err, "different players must be able to guess while a shared parent lock is held")
	require.NoError(t, lock.Rollback(t.Context()))
	resp, data = doJSON(t, "POST", path+"/comments", map[string]string{"content": "  Beautiful!  "}, viewer.access, nil)
	require.Equal(t, 201, resp.StatusCode)
	var comment models.PublicComment
	require.NoError(t, json.Unmarshal(data, &comment))
	require.Equal(t, "Beautiful!", comment.Content)
	resp, _ = doJSON(t, "DELETE", path+"/comments/"+comment.ID, nil, other.access, nil)
	require.Equal(t, 404, resp.StatusCode)
	resp, _ = doJSON(t, "DELETE", path+"/comments/"+comment.ID, nil, owner.access, nil)
	require.Equal(t, 204, resp.StatusCode)
	resp, _ = doJSON(t, "DELETE", path+"/reaction", nil, viewer.access, nil)
	require.Equal(t, 204, resp.StatusCode)
	resp, _ = doJSON(t, "DELETE", path, nil, viewer.access, nil)
	require.Equal(t, 404, resp.StatusCode)

	var key string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT storage_key FROM public_challenges WHERE id=$1`, publicID).Scan(&key))
	resp, _ = doJSON(t, "DELETE", path, nil, owner.access, nil)
	require.Equal(t, 204, resp.StatusCode)
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM media_deletion_jobs WHERE storage_key=$1`, key).Scan(&count))
	require.Equal(t, 1, count)
	resp, _ = doJSON(t, "GET", path+"/play", nil, other.access, nil)
	require.Equal(t, 404, resp.StatusCode)

	// Account deletion must enqueue the public object even through a cascade.
	second := uploadPublicPhoto(t, owner.access)
	require.NoError(t, db.QueryRow(t.Context(), `SELECT storage_key FROM public_challenges WHERE id=$1`, second).Scan(&key))
	_, err = db.Exec(t.Context(), `DELETE FROM users WHERE id=$1`, owner.userID)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM public_challenges WHERE id=$1`, second).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM media_deletion_jobs WHERE storage_key=$1`, key).Scan(&count))
	require.Equal(t, 1, count)
}
