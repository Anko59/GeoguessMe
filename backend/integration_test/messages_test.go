package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMessageCursorPagination(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Messages Group")
	joinGroup(t, bob.access, code)

	// Each uploaded challenge persists a chat message in the group.
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ids = append(ids, uploadPhoto(t, alice.access, groupID))
	}

	var page struct {
		Items []struct {
			ID      string `json:"id"`
			PhotoID string `json:"photo_id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	require.Eventually(t, func() bool {
		resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&limit=2", nil, alice.access, nil)
		if resp.StatusCode != http.StatusOK || jsonUnmarshal(data, &page) != nil {
			return false
		}
		return len(page.Items) == 2 && page.NextCursor != ""
	}, 5*time.Second, 100*time.Millisecond, "uploaded challenge messages must become queryable")

	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&limit=2&cursor="+page.NextCursor, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page2 struct {
		Items []struct {
			PhotoID string `json:"photo_id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, jsonUnmarshal(data, &page2))
	require.NotEmpty(t, page2.Items, "remaining messages must be returned")

	// Collect every photo id seen across both pages; all three challenges appear exactly once.
	seen := map[string]int{}
	for _, m := range page.Items {
		if m.PhotoID != "" {
			seen[m.PhotoID]++
		}
	}
	for _, m := range page2.Items {
		if m.PhotoID != "" {
			seen[m.PhotoID]++
		}
	}
	for _, id := range ids {
		require.Equalf(t, 1, seen[id], "challenge %s must appear exactly once across pages", id)
	}
}
