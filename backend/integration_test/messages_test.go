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
	bobName := unique("bob")
	bob := signup(t, bobName, bobName+"@example.test", "StrongPassword123")
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
	resp, data := doJSON(t, http.MethodPut, path, map[string]string{"reaction": "like"}, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var updated struct {
		Reactions []struct {
			Reaction  string   `json:"reaction"`
			Count     int      `json:"count"`
			Reacted   bool     `json:"reacted"`
			Usernames []string `json:"usernames"`
		} `json:"reactions"`
	}
	require.NoError(t, json.Unmarshal(data, &updated))
	require.Equal(t, "like", updated.Reactions[0].Reaction)
	require.Equal(t, 1, updated.Reactions[0].Count)
	require.True(t, updated.Reactions[0].Reacted)
	require.Equal(t, []string{bobName}, updated.Reactions[0].Usernames)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, livePayload, err := conn.ReadMessage()
	require.NoError(t, err)
	var liveUpdate struct {
		ReactionUpdate struct {
			UserID   string `json:"user_id"`
			Reaction string `json:"reaction"`
			Active   bool   `json:"active"`
		} `json:"reaction_update"`
	}
	require.NoError(t, json.Unmarshal(livePayload, &liveUpdate))
	require.Equal(t, bob.userID, liveUpdate.ReactionUpdate.UserID)
	require.Equal(t, "like", liveUpdate.ReactionUpdate.Reaction)
	require.True(t, liveUpdate.ReactionUpdate.Active)

	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Items []struct {
			Reactions []struct {
				Count     int      `json:"count"`
				Usernames []string `json:"usernames"`
			} `json:"reactions"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, 1, page.Items[0].Reactions[0].Count)
	require.Equal(t, []string{bobName}, page.Items[0].Reactions[0].Usernames)

	resp, data = doJSON(t, http.MethodDelete, path, map[string]string{"reaction": "like"}, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var removed struct {
		Reactions []struct {
			Reaction string `json:"reaction"`
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

func TestMessageCursorPaginationBackward(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Backward Pagination Group")

	// Seed six text messages over the WebSocket; sequential sends get
	// increasing server timestamps so ordering by (created_at, id) is stable.
	conn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer conn.Close()
	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		require.NoError(t, conn.WriteJSON(map[string]string{"content": "message"}))
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, payload, err := conn.ReadMessage()
		require.NoError(t, err, "broadcast of seeded message %d", i+1)
		var sent struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(payload, &sent))
		require.NotEmpty(t, sent.ID)
		ids = append(ids, sent.ID)
	}

	type item struct {
		ID string `json:"id"`
	}
	type page struct {
		Items      []item `json:"items"`
		NextCursor string `json:"next_cursor"`
	}

	latest := page{}
	require.Eventually(t, func() bool {
		resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&limit=2", nil, alice.access, nil)
		if resp.StatusCode != http.StatusOK || jsonUnmarshal(data, &latest) != nil {
			return false
		}
		return len(latest.Items) == 2
	}, 5*time.Second, 100*time.Millisecond, "latest page must become queryable")
	require.Equal(t, []string{ids[4], ids[5]}, []string{latest.Items[0].ID, latest.Items[1].ID}, "latest page is the newest two messages")

	older := page{}
	require.Eventually(t, func() bool {
		resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&before_id="+latest.Items[0].ID+"&limit=2", nil, alice.access, nil)
		if resp.StatusCode != http.StatusOK || jsonUnmarshal(data, &older) != nil {
			return false
		}
		return len(older.Items) == 2
	}, 5*time.Second, 100*time.Millisecond, "older page must become queryable")
	require.Equal(t, []string{ids[2], ids[3]}, []string{older.Items[0].ID, older.Items[1].ID}, "older page is the two messages before the latest page")

	oldest := page{}
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&before_id="+older.Items[0].ID+"&limit=2", nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, jsonUnmarshal(data, &oldest))
	require.Equal(t, []string{ids[0], ids[1]}, []string{oldest.Items[0].ID, oldest.Items[1].ID}, "draining page is the first two messages")

	exhausted := page{}
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+"&before_id="+oldest.Items[0].ID+"&limit=2", nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, jsonUnmarshal(data, &exhausted))
	require.Empty(t, exhausted.Items, "before the first message there is nothing older")
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
	accepted := deliverChallengeMedia(t, bob.access, acceptChallenge(t, bob.access, photoID))
	require.Equal(t, "accepted", messageStatus(t, bob.access), "accepted participant sees Continue challenge")

	conn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]string{"content": "ready"}))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err := conn.ReadMessage()
	require.NoError(t, err, "socket must be registered before the guess update")

	waitUntilViewExpires(t, accepted.ViewExpiresAt)
	require.Equal(t, http.StatusCreated, guess(t, bob.access, photoID, 48.8, 2.3))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	var update struct {
		ID                string `json:"id"`
		PhotoID           string `json:"photo_id"`
		ChallengeResolved bool   `json:"challenge_resolved"`
	}
	require.NoError(t, json.Unmarshal(payload, &update))
	require.Equal(t, photoID, update.PhotoID)
	require.True(t, update.ChallengeResolved, "open conversations receive the resolved state immediately")
}

func TestContentReportsMembershipAndIdempotency(t *testing.T) {
	alice := signup(t, unique("reporter"), unique("reporter")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("target"), unique("target")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("outsider"), unique("outsider")+"@example.test", "StrongPassword123")
	groupID, invite := createGroup(t, alice.access, "Report membership")
	joinGroup(t, bob.access, invite)
	body := map[string]string{"reason": "harassment", "details": "context"}
	path := "/api/v1/users/" + bob.userID + "/report"
	resp, _ := doJSON(t, http.MethodPost, path, body, outsider.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/users/"+alice.userID+"/report", body, alice.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, data := doJSON(t, http.MethodPost, path, body, alice.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "%s", data)
	var receipt struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &receipt))
	require.NotEmpty(t, receipt.ID)
	resp, data = doJSON(t, http.MethodPost, path, map[string]string{"reason": "other"}, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var duplicate struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &duplicate))
	require.Equal(t, receipt.ID, duplicate.ID)
	db := testDB(t)
	var reportCount int
	var recordedReason, recordedDetails, recordedTarget string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM content_reports WHERE reporter_id = $1 AND target_kind = 'user' AND target_id = $2`, alice.userID, bob.userID).Scan(&reportCount))
	require.Equal(t, 1, reportCount)
	require.NoError(t, db.QueryRow(t.Context(), `SELECT reason, details, reported_user_id FROM content_reports WHERE id = $1`, receipt.ID).Scan(&recordedReason, &recordedDetails, &recordedTarget))
	require.Equal(t, "harassment", recordedReason)
	require.Equal(t, "context", recordedDetails)
	require.Equal(t, bob.userID, recordedTarget)

	conn := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]string{"content": "report target"}))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var message struct {
		ID string `json:"id"`
	}
	require.NoError(t, conn.ReadJSON(&message))
	require.NotEmpty(t, message.ID)
	messagePath := "/api/v1/messages/" + message.ID + "/report"
	resp, _ = doJSON(t, http.MethodPost, messagePath, body, outsider.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, data = doJSON(t, http.MethodPost, messagePath, body, alice.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "%s", data)
	resp, _ = doJSON(t, http.MethodPost, messagePath, body, bob.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBlockingAuthoritativeHistoryLiveAndProfiles(t *testing.T) {
	alice := signup(t, unique("blockalice"), unique("blockalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("blockbob"), unique("blockbob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("blockcarol"), unique("blockcarol")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("blockoutsider"), unique("blockoutsider")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Blocking Group")
	joinGroup(t, bob.access, code)
	joinGroup(t, carol.access, code)
	aliceConn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceConn.Close()
	bobConn := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobConn.Close()
	carolConn := mustDialWS(t, groupID, wsTicket(t, carol.access, groupID), baseURL)
	defer carolConn.Close()
	read := func(conn interface {
		SetReadDeadline(time.Time) error
		ReadJSON(any) error
	}) map[string]any {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		var message map[string]any
		require.NoError(t, conn.ReadJSON(&message))
		return message
	}
	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "before block"}))
	before := read(bobConn)
	require.Equal(t, "before block", read(aliceConn)["content"])
	require.Equal(t, "before block", read(carolConn)["content"])
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	var page struct {
		Items        []map[string]any `json:"items"`
		StableCursor string           `json:"stable_cursor"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	cursor := page.StableCursor
	path := "/api/v1/users/" + bob.userID + "/block"
	resp, _ = doJSON(t, http.MethodPost, path, nil, outsider.access, nil)
	require.Equal(t, 404, resp.StatusCode)
	// Concurrent identical requests must all succeed and preserve one row/time.
	statuses := make(chan int, 4)
	for range 4 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+path, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+alice.access)
		go func() {
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	for range 4 {
		require.Equal(t, 204, <-statuses)
	}
	resp, data = doJSON(t, http.MethodGet, "/api/v1/users/blocks", nil, alice.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &list))
	require.Len(t, list.Items, 1)
	require.Equal(t, bob.userID, list.Items[0]["user_id"])
	require.NotContains(t, list.Items[0], "email")
	resp, data = doJSON(t, http.MethodGet, "/api/v1/users/blocks", nil, bob.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.Unmarshal(data, &list))
	require.Empty(t, list.Items)
	for _, tc := range []struct{ viewer, target string }{{alice.access, bob.userID}, {bob.access, alice.userID}} {
		for _, suffix := range []string{"/api/v1/user/profile/", "/api/v1/users/"} {
			url := suffix + tc.target
			if suffix == "/api/v1/users/" {
				url += "/avatar"
			}
			resp, _ = doJSON(t, http.MethodGet, url, nil, tc.viewer, nil)
			require.Equal(t, 404, resp.StatusCode)
		}
	}
	// State-based sentinel: Bob's echo proves persistence and fanout completed;
	// Carol's next event proves Alice's socket remains alive without a timeout.
	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "hidden live"}))
	require.Equal(t, "hidden live", read(bobConn)["content"])
	require.Equal(t, "hidden live", read(carolConn)["content"])
	require.NoError(t, carolConn.WriteJSON(map[string]string{"content": "visible sentinel"}))
	marker := read(carolConn)
	require.Equal(t, "visible sentinel", read(aliceConn)["content"])
	require.Equal(t, "visible sentinel", read(bobConn)["content"])
	for _, query := range []string{"", "&cursor=" + cursor, "&before_id=" + marker["id"].(string)} {
		resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID+query, nil, alice.access, nil)
		require.Equal(t, 200, resp.StatusCode, string(data))
		require.NoError(t, json.Unmarshal(data, &page))
		for _, item := range page.Items {
			require.NotEqual(t, bob.userID, item["user_id"])
		}
	}
	resp, _ = doJSON(t, http.MethodPut, "/api/v1/group/message-reactions/"+before["id"].(string), map[string]string{"reaction": "like"}, alice.access, nil)
	require.Equal(t, 404, resp.StatusCode)
	for range 2 {
		resp, _ = doJSON(t, http.MethodDelete, path, nil, alice.access, nil)
		require.Equal(t, 204, resp.StatusCode)
	}
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(data), "hidden live")
	resp, _ = doJSON(t, http.MethodPost, path, nil, alice.access, nil)
	require.Equal(t, 204, resp.StatusCode)
	db := testDB(t)
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT COUNT(*) FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`, alice.userID, bob.userID).Scan(&count))
	require.Equal(t, 1, count)
	_, err := db.Exec(t.Context(), `INSERT INTO user_blocks(blocker_id,blocked_id) VALUES ($1,$1)`, alice.userID)
	require.Error(t, err, "database must reject self blocking")
	_, err = db.Exec(t.Context(), `DELETE FROM group_members WHERE group_id=$1 AND user_id=$2`, groupID, bob.userID)
	require.NoError(t, err)
	resp, _ = doJSON(t, http.MethodDelete, path, nil, alice.access, nil)
	require.Equal(t, 204, resp.StatusCode, "unblock must not require current shared membership")
	// Restore the directed preference using an existing row, then delete the
	// target: migration foreign keys must clear both preference directions.
	_, err = db.Exec(t.Context(), `INSERT INTO user_blocks(blocker_id,blocked_id) VALUES ($1,$2),($2,$1)`, alice.userID, bob.userID)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), `DELETE FROM users WHERE id=$1`, bob.userID)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(t.Context(), `SELECT COUNT(*) FROM user_blocks WHERE blocker_id=$1 OR blocked_id=$1`, bob.userID).Scan(&count))
	require.Zero(t, count)
}
