package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMessageReactionsAreScopedAndToggleable(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Reaction Group")
	joinGroup(t, bob.access, code)

	conn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]string{"content": "React to this"}))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	var sent struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(payload, &sent))
	require.NotEmpty(t, sent.ID)

	path := "/api/v1/group/message-reactions/" + sent.ID
	resp, data := doJSON(t, http.MethodPut, path, map[string]string{"emoji": "👍"}, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var updated struct {
		Reactions []struct {
			Emoji   string `json:"emoji"`
			Count   int    `json:"count"`
			Reacted bool   `json:"reacted"`
		} `json:"reactions"`
	}
	require.NoError(t, json.Unmarshal(data, &updated))
	require.Equal(t, "👍", updated.Reactions[0].Emoji)
	require.Equal(t, 1, updated.Reactions[0].Count)
	require.True(t, updated.Reactions[0].Reacted)

	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Items []struct {
			Reactions []struct {
				Count int `json:"count"`
			} `json:"reactions"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, 1, page.Items[0].Reactions[0].Count)

	resp, data = doJSON(t, http.MethodDelete, path, map[string]string{"emoji": "👍"}, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var removed struct {
		Reactions []struct {
			Emoji string `json:"emoji"`
		} `json:"reactions"`
	}
	require.NoError(t, json.Unmarshal(data, &removed))
	require.Empty(t, removed.Reactions)

}

func TestMessageCursorPagination(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Messages Group")
	joinGroup(t, bob.access, code)

	// Each uploaded challenge persists a chat message in the group. Uploads are
	// sequential so their server timestamps increase with upload order.
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ids = append(ids, uploadPhoto(t, alice.access, groupID))
	}

	// The latest page (empty cursor) must expose every message chronologically
	// with no forward cursor because nothing is newer.
	var full struct {
		Items []struct {
			ID      string `json:"id"`
			PhotoID string `json:"photo_id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	require.Eventually(t, func() bool {
		resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
		if resp.StatusCode != http.StatusOK || jsonUnmarshal(data, &full) != nil {
			return false
		}
		return len(full.Items) == 3 && full.NextCursor == ""
	}, 5*time.Second, 100*time.Millisecond, "uploaded challenge messages must become queryable on the latest page")

	// Every uploaded challenge appears exactly once on the full latest page.
	seen := map[string]int{}
	for _, m := range full.Items {
		if m.PhotoID != "" {
			seen[m.PhotoID]++
		}
	}
	for _, id := range ids {
		require.Equalf(t, 1, seen[id], "challenge %s must appear exactly once", id)
	}

	// A smaller limit returns only the most recent messages in chronological
	// order, with no forward cursor: the page is the tail of the full list.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&limit=2", nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var recent struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, jsonUnmarshal(data, &recent))
	require.Len(t, recent.Items, 2, "latest page must respect the limit")
	require.Empty(t, recent.NextCursor, "latest page must have no forward cursor")
	require.Equal(t, full.Items[1].ID, recent.Items[0].ID, "latest page must start at the second-newest message")
	require.Equal(t, full.Items[2].ID, recent.Items[1].ID, "latest page must end at the newest message")
}

func TestChallengeMessageStatusIsViewerSpecific(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Challenge Status Group")
	joinGroup(t, bob.access, code)
	photoID := uploadPhoto(t, alice.access, groupID)

	messageStatus := func(t *testing.T, bearer string) string {
		t.Helper()
		resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, bearer, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var page struct {
			Items []struct {
				PhotoID         string `json:"photo_id"`
				ChallengeStatus string `json:"challenge_status"`
			} `json:"items"`
		}
		require.NoError(t, jsonUnmarshal(data, &page))
		for _, item := range page.Items {
			if item.PhotoID == photoID {
				return item.ChallengeStatus
			}
		}
		return ""
	}

	require.Eventually(t, func() bool { return messageStatus(t, alice.access) == "results" }, 5*time.Second, 100*time.Millisecond, "uploader status must be available once the challenge message is persisted")
	require.Equal(t, "available", messageStatus(t, bob.access), "participant starts with Accept challenge")
	acceptChallenge(t, bob.access, photoID)
	require.Equal(t, "accepted", messageStatus(t, bob.access), "accepted participant sees Continue challenge")
}
