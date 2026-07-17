package integration_test

import (
	"bytes"
	"context"
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
	require.NoError(t, writer.WriteField("group_id", groupID))
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
}

type leaderboardEntry struct {
	UserID     string  `json:"user_id"`
	Username   string  `json:"username"`
	Score      int     `json:"score"`
	GuessCount int     `json:"guess_count"`
	Average    float64 `json:"average_score"`
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
	acc := acceptChallenge(t, bob.access, photoA)
	waitUntilViewExpires(t, acc.ViewExpiresAt)
	require.Equal(t, http.StatusCreated, guess(t, bob.access, photoA, 51.5, -0.1))

	aEntries := leaderboard(t, alice.access, groupA)
	bEntries := leaderboard(t, carol.access, groupB)

	bobA := findEntry(aEntries, "bob")
	require.NotNil(t, bobA, "bob must be a Group A member")
	require.Equal(t, 1, bobA.GuessCount)
	require.Greater(t, bobA.Score, 0)

	bobB := findEntry(bEntries, "bob")
	require.NotNil(t, bobB, "bob must be a Group B member")
	require.Equal(t, 0, bobB.GuessCount, "Group A guess count must not leak into Group B")
	require.Equal(t, 0, bobB.Score)
}
