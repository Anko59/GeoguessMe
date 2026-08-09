package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func attemptUpload(t *testing.T, bearer, groupID string) int {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("group_ids", groupID))
	require.NoError(t, writer.WriteField("lat", "1"))
	require.NoError(t, writer.WriteField("long", "1"))
	require.NoError(t, writer.Close())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/api/v1/photo/upload", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func uploadGroupPhoto(t *testing.T, bearer, groupID string) (jsonResponse, []byte) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "group.png")
	require.NoError(t, err)
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("group_id", groupID))
	require.NoError(t, writer.Close())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/group/photo", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return jsonResponse{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies()}, data
}

func TestNonMemberForbiddenMatrix(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("out"), unique("out")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Private Group")
	photoID := uploadPhoto(t, alice.access, groupID)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"details", http.MethodGet, "/api/v1/group/details?id=" + groupID, nil},
		{"members", http.MethodGet, "/api/v1/group/members?id=" + groupID, nil},
		{"messages", http.MethodGet, "/api/v1/group/messages?group_id=" + groupID, nil},
		{"leaderboard", http.MethodGet, "/api/v1/group/leaderboard?group_id=" + groupID, nil},
		{"group_photo", http.MethodGet, "/api/v1/group/photo?group_id=" + groupID, nil},
		{"group_notifications", http.MethodGet, "/api/v1/group/notifications?group_id=" + groupID, nil},
		{"ws_ticket", http.MethodPost, "/api/v1/ws/ticket?group_id=" + groupID, map[string]string{}},
		{"accept", http.MethodPost, "/api/v1/challenges/" + photoID + "/accept", map[string]string{}},
		{"guess", http.MethodPost, "/api/v1/challenges/" + photoID + "/guess", map[string]float64{"lat": 0, "long": 0}},
		{"media", http.MethodGet, "/api/v1/challenges/" + photoID + "/media", nil},
		{"results", http.MethodGet, "/api/v1/challenges/" + photoID + "/results", nil},
	}
	for _, tc := range cases {
		resp, _ := doJSON(t, tc.method, tc.path, tc.body, outsider.access, nil)
		require.Equalf(t, http.StatusForbidden, resp.StatusCode, tc.name)
	}
	require.Equal(t, http.StatusForbidden, attemptUpload(t, outsider.access, groupID), "upload")
	photoResp, _ := uploadGroupPhoto(t, outsider.access, groupID)
	require.Equal(t, http.StatusForbidden, photoResp.StatusCode, "group photo upload")
}

func TestGroupPhotoAndNotificationSettings(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("out"), unique("out")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Settings Group")

	photoResp, _ := uploadGroupPhoto(t, alice.access, groupID)
	require.Equal(t, http.StatusOK, photoResp.StatusCode)
	photoResp, photoData := doJSON(t, http.MethodGet, "/api/v1/group/photo?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, photoResp.StatusCode)
	require.Equal(t, "image/jpeg", photoResp.Header.Get("Content-Type"))
	require.NotEmpty(t, photoData)

	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/notifications?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var preference struct {
		Enabled bool `json:"enabled"`
	}
	require.NoError(t, jsonUnmarshal(data, &preference))
	require.True(t, preference.Enabled, "new members should receive notifications by default")

	resp, data = doJSON(t, http.MethodPut, "/api/v1/group/notifications?group_id="+groupID, map[string]bool{"enabled": false}, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, jsonUnmarshal(data, &preference))
	require.False(t, preference.Enabled)
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/notifications?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, jsonUnmarshal(data, &preference))
	require.False(t, preference.Enabled)

	resp, _ = doJSON(t, http.MethodGet, "/api/v1/group/photo?group_id="+groupID, nil, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPut, "/api/v1/group/notifications?group_id="+groupID, map[string]bool{"enabled": true}, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

type leaderboardEntry struct {
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	Avatar      string  `json:"avatar"`
	Score       int     `json:"score"`
	GuessCount  int     `json:"guess_count"`
	Average     float64 `json:"average_score"`
	TotalPoints int     `json:"total_points"`
	Rank        struct {
		Level int    `json:"level"`
		Name  string `json:"name"`
	} `json:"rank"`
}

func leaderboard(t *testing.T, bearer, groupID string) []leaderboardEntry {
	t.Helper()
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupID, nil, bearer, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var entries []leaderboardEntry
	require.NoError(t, jsonUnmarshal(data, &entries))
	return entries
}

func findEntry(entries []leaderboardEntry, prefix string) *leaderboardEntry {
	for i := range entries {
		if strings.HasPrefix(entries[i].Username, prefix) {
			return &entries[i]
		}
	}
	return nil
}

func TestCrossGroupIsolation(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("carol"), unique("carol")+"@example.test", "StrongPassword123")

	groupA, codeA := createGroup(t, alice.access, "Group A")
	groupB, codeB := createGroup(t, carol.access, "Group B")
	joinGroup(t, bob.access, codeA)
	joinGroup(t, bob.access, codeB)

	photoA := uploadPhoto(t, alice.access, groupA)
	acc := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoA))
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	require.Equal(t, http.StatusCreated, guess(t, bob.access, photoA, 51.5, -0.1))

	aEntries := leaderboard(t, alice.access, groupA)
	bEntries := leaderboard(t, carol.access, groupB)

	bobA := findEntry(aEntries, "bob")
	require.NotNil(t, bobA, "bob must be a Group A member")
	require.NotEmpty(t, bobA.Avatar, "leaderboard entries must include the member avatar")
	require.Equal(t, 1, bobA.GuessCount)
	require.Greater(t, bobA.Score, 0)
	require.Greater(t, bobA.TotalPoints, 0)
	require.NotEmpty(t, bobA.Rank.Name)

	bobB := findEntry(bEntries, "bob")
	require.NotNil(t, bobB, "bob must be a Group B member")
	require.Equal(t, 0, bobB.GuessCount, "Group A guess count must not leak into Group B")
	require.Equal(t, 0, bobB.Score)
}
