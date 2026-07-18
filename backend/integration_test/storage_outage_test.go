package integration_test

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRealStorageOutage exercises a real object-storage outage by adding a
// Toxiproxy timeout toxic that drops all downstream connections to MinIO
// with an immediate timeout. It proves:
//  1. /health/ready returns 503 while storage is unreachable.
//  2. Upload requests return 502 with the documented storage_error code.
//  3. No photo rows are committed while storage is down.
//  4. After the toxic is removed, /health/ready recovers to 200 through
//     state polling and a fresh upload plus media retrieval succeed.
func TestRealStorageOutage(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Outage Group")
	joinGroup(t, bob.access, code)

	// Baseline: a healthy upload and a known photo count.
	baselineID := uploadPhoto(t, alice.access, groupID)
	require.NotEmpty(t, baselineID)
	before := countPhotos(t)
	require.GreaterOrEqual(t, before, 1)

	// Simulate storage outage via a timeout toxic on the Toxiproxy proxy.
	addToxiproxyTimeout(t, "minio", 0)

	// /health/ready must reflect the storage outage.
	waitForReadyStatus(t, http.StatusServiceUnavailable)

	// Uploads must fail with the documented storage_error code. Use a raw
	// multipart request so we can inspect the status without the helper's
	// built-in assertion.
	resp, data := tryUploadPhoto(t, alice.access, groupID)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "upload must return 502 during storage outage")
	require.Contains(t, string(data), "storage_error", "upload error must document storage_error")

	// No photo row may be committed while storage is unreachable.
	require.Equal(t, before, countPhotos(t), "photo count must not change during outage")

	// Restore storage connectivity by removing the timeout toxic.
	removeToxiproxyTimeout(t, "minio")

	// Poll until /health/ready reports healthy again.
	waitForReadyStatus(t, http.StatusOK)

	// A fresh upload must succeed after recovery.
	recoveryID := uploadPhoto(t, alice.access, groupID)
	require.NotEmpty(t, recoveryID)

	// The recovered upload must be retrievable through the media endpoint.
	db := testDB(t)
	_, err := db.Exec(t.Context(),
		`INSERT INTO challenge_views (photo_id, user_id, accepted_at, view_expires_at) VALUES ($1, $2, NOW(), NOW() + interval '5 minutes') ON CONFLICT DO NOTHING`,
		recoveryID, bob.userID)
	require.NoError(t, err)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+recoveryID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "media must be served after storage recovery")
}

// tryUploadPhoto sends a multipart photo upload without asserting status,
// returning the raw response for outage/failure assertions.
func tryUploadPhoto(t *testing.T, bearer, groupID string) (jsonResponse, []byte) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.png")
	require.NoError(t, err)
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("lat", "51.505"))
	require.NoError(t, writer.WriteField("long", "-0.09"))
	require.NoError(t, writer.WriteField("group_id", groupID))
	require.NoError(t, writer.Close())

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/photo/upload", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return jsonResponse{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies()}, data
}
