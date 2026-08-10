package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// processingJobResponse is the owner-facing media-processing job payload.
type processingJobResponse struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	QueuedAt  string `json:"queued_at"`
	ErrorCode string `json:"error_code"`
}

// uploadChatMediaVideo uploads a malformed WebM clip. F-10 quarantines the
// raw bytes and queues a media-processing job (202) instead of serving the
// video synchronously as chat media.
func uploadChatMediaVideo(t *testing.T, bearer, groupID, content string) processingJobResponse {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("media", "clip.webm")
	require.NoError(t, err)
	_, err = part.Write([]byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'})
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("group_id", groupID))
	require.NoError(t, writer.WriteField("content", content))
	require.NoError(t, writer.Close())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/group/messages/media", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusAccepted, resp.StatusCode, "chat-media video upload %d: %s", resp.StatusCode, data)
	var job processingJobResponse
	require.NoError(t, json.Unmarshal(data, &job))
	return job
}

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

	// F-10: video uploads are asynchronous. The raw clip is quarantined and a
	// media-processing job is returned (202); it is never synchronously served
	// as chat media, and the malformed clip is deterministically failed by the
	// in-process worker with a stable, non-sensitive error code.
	video := uploadChatMediaVideo(t, alice.access, groupID, "A short clip")
	require.Equal(t, "chat", video.Kind)
	require.Equal(t, "queued", video.Status)
	require.NotEmpty(t, video.ID)

	// The quarantined raw clip must never be reachable as chat media.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/group/messages/media/"+video.ID, nil, bob.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "quarantined video must never be served as chat media")

	// Job identifiers cannot be enumerated: a non-owner gets the same 404 as a
	// missing job.
	statusURL := "/api/v1/media-processing/" + video.ID
	resp, _ = doJSON(t, http.MethodGet, statusURL, nil, outsider.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "non-owners must never read processing status")

	// The in-process worker claims the job, ffprobe rejects the malformed
	// clip, and the job reaches the failed state with the stable error code.
	var job processingJobResponse
	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, data = doJSON(t, http.MethodGet, statusURL, nil, alice.access, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, json.Unmarshal(data, &job))
		if job.Status == "failed" || job.Status == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("video job did not reach a terminal state: %s", data)
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.Equal(t, "failed", job.Status)
	require.Equal(t, "invalid_video", job.ErrorCode)
}
