package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatMediaIsPersistedBroadcastAndPrivate(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("outsider"), unique("outsider")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Chat Media Group")
	joinGroup(t, bob.access, code)

	bobConn := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobConn.Close()
	sent := uploadChatMedia(t, alice.access, groupID, "Look at this")
	require.Equal(t, "media", sent.Kind)
	require.NotEmpty(t, sent.ID)
	require.NotEmpty(t, sent.MediaID)
	require.Equal(t, "image/png", sent.MediaType)

	require.NoError(t, bobConn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var broadcast chatMediaMessage
	require.NoError(t, bobConn.ReadJSON(&broadcast))
	require.Equal(t, sent.ID, broadcast.ID)
	require.Equal(t, sent.MediaID, broadcast.MediaID)

	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(data), sent.MediaID, "media message must survive reconnect/history load")

	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages/media/"+sent.MediaID, nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	require.Equal(t, "private, no-store", resp.Header.Get("Cache-Control"))
	require.NotEmpty(t, data)

	resp, _ = doJSON(t, http.MethodGet, "/api/v1/group/messages/media/"+sent.MediaID, nil, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "non-members must never read chat attachments")

	video := uploadChatMediaFile(t, alice.access, groupID, "A short clip", "clip.webm", []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'})
	require.Equal(t, "media", video.Kind)
	require.Equal(t, "video/webm", video.MediaType)
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/messages/media/"+video.MediaID, nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "video/webm", resp.Header.Get("Content-Type"))
	require.NotEmpty(t, data)
}
