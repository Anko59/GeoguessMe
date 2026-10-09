package integration_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"geoguessme/internal/models"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func uploadChallengeWithOptions(t *testing.T, bearer string, groupIDs []string, hideLocation bool) map[string]any {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.png")
	require.NoError(t, err)
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	for _, groupID := range groupIDs {
		require.NoError(t, writer.WriteField("group_ids", groupID))
	}
	require.NoError(t, writer.WriteField("hide_location", boolString(hideLocation)))
	require.NoError(t, writer.WriteField("lat", "51.505"))
	require.NoError(t, writer.WriteField("long", "-0.09"))
	require.NoError(t, writer.Close())

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/photo/upload", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "upload %d: %s", resp.StatusCode, data)
	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func TestMultiGroupUploadAndHiddenLocation(t *testing.T) {
	alice := signup(t, unique("mlgalice"), unique("mlgalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("mlgbob"), unique("mlgbob")+"@example.test", "StrongPassword123")

	groupOne, groupOneCode := createGroup(t, alice.access, "Multi one")
	groupTwo, _ := createGroup(t, alice.access, "Multi two")
	joinGroup(t, bob.access, groupOneCode)

	// Alice sends one challenge to two groups and hides her location.
	uploaded := uploadChallengeWithOptions(t, alice.access, []string{groupOne, groupTwo}, true)
	photos, ok := uploaded["photos"].([]any)
	require.True(t, ok, "upload response must carry a photos list: %v", uploaded)
	require.Len(t, photos, 2)

	// Bob sees the challenge in his group (the chat broadcast persists
	// asynchronously, so poll until it lands).
	var photoID string
	uploadedIDs := strings.Join(photoIDsFrom(uploaded), ",")
	require.Eventually(t, func() bool {
		for _, message := range fetchGroupMessages(t, bob.access, groupOne) {
			if message.Kind == "challenge" && message.PhotoID != "" && strings.Contains(uploadedIDs, message.PhotoID) {
				photoID = message.PhotoID
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "bob must receive the challenge in his group")

	// Bob accepts, guesses, and sees distances but not the exact location.
	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
	mediaResp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, mediaResp.StatusCode)
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	guess(t, bob.access, photoID, 10, 10)

	resp, data := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/results", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var results struct {
		LocationHidden bool     `json:"location_hidden"`
		ActualLat      *float64 `json:"actual_lat"`
		Guesses        []struct {
			Distance float64 `json:"distance"`
		} `json:"guesses"`
	}
	require.NoError(t, json.Unmarshal(data, &results))
	require.True(t, results.LocationHidden)
	require.Nil(t, results.ActualLat, "the exact location must stay hidden")
	require.Len(t, results.Guesses, 1)
	require.Greater(t, results.Guesses[0].Distance, 0.0, "guesses still report their distance")
}

func photoIDsFrom(uploaded map[string]any) []string {
	photos, _ := uploaded["photos"].([]any)
	ids := make([]string, 0, len(photos))
	for _, photo := range photos {
		entry, _ := photo.(map[string]any)
		if id, ok := entry["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func fetchGroupMessages(t *testing.T, bearer, groupID string) []struct {
	Kind    string `json:"kind"`
	PhotoID string `json:"photo_id"`
} {
	t.Helper()
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, bearer, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var page struct {
		Items []struct {
			Kind    string `json:"kind"`
			PhotoID string `json:"photo_id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	return page.Items
}

func TestPublicLocationPrivacyAcrossTimedAndLegacyReads(t *testing.T) {
	owner := signup(t, unique("ppowner"), unique("ppowner")+"@example.test", "StrongPassword123")
	viewer := signup(t, unique("ppview"), unique("ppview")+"@example.test", "StrongPassword123")
	peer := signup(t, unique("pppeer"), unique("pppeer")+"@example.test", "StrongPassword123")
	id := uploadPublicPhotoWithPrivacy(t, owner.access, true)
	path := "/api/v1/feed/challenges/" + id
	for _, player := range []tokenPair{viewer, peer} {
		resp, data := doJSON(t, http.MethodPost, path+"/guess", map[string]float64{"lat": 1, "long": 2}, player.access, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		require.NotContains(t, string(data), "actual_lat")
		require.Contains(t, string(data), `"location_hidden":true`)
	}
	resp, data := doJSON(t, http.MethodGet, path+"/guess", nil, viewer.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, string(data), "actual_lat")
	resp, data = doJSON(t, http.MethodGet, path+"/timed-results", nil, viewer.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	require.NotContains(t, string(data), "actual_lat")
	var result models.PublicTimedResults
	require.NoError(t, json.Unmarshal(data, &result))
	require.True(t, result.LocationHidden)
	require.NotNil(t, result.LocationRevealsAt)
	require.Len(t, result.Guesses, 2)
	for _, guess := range result.Guesses {
		if guess.UserID == viewer.userID {
			require.NotNil(t, guess.Lat)
			require.NotNil(t, guess.Distance)
		} else {
			require.Nil(t, guess.Lat)
			require.Nil(t, guess.Long)
			require.Nil(t, guess.Distance)
		}
	}
	resp, data = doJSON(t, http.MethodGet, path+"/results", nil, viewer.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var legacy []models.PublicFeedResult
	require.NoError(t, json.Unmarshal(data, &legacy))
	require.Len(t, legacy, 2)
	for _, guess := range legacy {
		if guess.IsViewer {
			require.NotNil(t, guess.Distance)
		} else {
			require.Nil(t, guess.Distance)
		}
	}
	resp, data = doJSON(t, http.MethodGet, path+"/timed-results", nil, owner.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(data), "actual_lat")
	_, err := testDB(t).Exec(t.Context(), `UPDATE public_challenges SET created_at=clock_timestamp()-interval '49 hours' WHERE id=$1`, id)
	require.NoError(t, err)
	resp, data = doJSON(t, http.MethodGet, path+"/timed-results", nil, viewer.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	result = models.PublicTimedResults{}
	require.NoError(t, json.Unmarshal(data, &result))
	require.False(t, result.LocationHidden)
	require.Nil(t, result.LocationRevealsAt)
	require.NotNil(t, result.ActualLat)
	require.InDelta(t, 48.8, *result.ActualLat, 0.000001)
	require.NotContains(t, string(data), "location_hidden")
	for _, guess := range result.Guesses {
		require.NotNil(t, guess.Lat)
		require.NotNil(t, guess.Distance)
	}
}
