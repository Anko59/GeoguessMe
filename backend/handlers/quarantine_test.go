package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v4"
)

// TestQuarantineKeysAreUnservable proves the media serving handlers reject any
// storage key outside the canonical prefixes. Raw quarantine bytes are planted
// at a quarantine key; even a database row that referenced that key (which
// should never happen because uploads only write canonical prefixes) must not
// stream them.
func TestQuarantineKeysAreUnservable(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("raw quarantine bytes")
	if err := store.Put(context.Background(), "quarantine/raw-uuid", bytes.NewReader(raw), int64(len(raw)), "video/mp4"); err != nil {
		t.Fatal(err)
	}

	groupID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Chat media: a row referencing a quarantine key must answer gone without
	// streaming the raw object.
	mock := newMockPool(t)
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	chatAPI := newChatAPI(t, mock, store, hub)
	asset := &models.ChatMedia{ID: "00000000-0000-0000-0000-000000000002", GroupID: groupID, UserID: "user-2", StorageKey: "quarantine/raw-uuid", MIMEType: "video/mp4", ByteSize: int64(len(raw)), CreatedAt: now}
	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(asset.ID).WillReturnRows(
		pgxmock.NewRows([]string{"group_id", "user_id", "storage_key", "mime_type", "byte_size", "created_at"}).AddRow(asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt),
	)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	recorder := httptest.NewRecorder()
	req := requestWithUser(http.MethodGet, "/", "", "user-1")
	req.SetPathValue("mediaID", asset.ID)
	chatAPI.ServeChatMedia(recorder, req)
	if recorder.Code != http.StatusGone {
		t.Fatalf("chat quarantine media status = %d, want 410 (%s)", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() == string(raw) {
		t.Fatal("ServeChatMedia streamed raw quarantine bytes")
	}

	// Challenge media: the same guard protects the photo serving path.
	mock2 := newMockPool(t)
	gameAPI := newGameAPI(t, mock2)
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000003", UserID: "user-2", GroupID: groupID, StorageKey: "quarantine/raw-uuid", MIMEType: "video/mp4", ByteSize: int64(len(raw)), Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock2.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock2.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock2.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(nil, now.Add(time.Hour)))
	recorder = httptest.NewRecorder()
	req = requestWithUser(http.MethodGet, "/", "", "user-1")
	req.SetPathValue("photoID", photo.ID)
	gameAPI.ServeChallengeMedia(recorder, req)
	if recorder.Code != http.StatusGone {
		t.Fatalf("challenge quarantine media status = %d, want 410 (%s)", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() == string(raw) {
		t.Fatal("ServeChallengeMedia streamed raw quarantine bytes")
	}
}
