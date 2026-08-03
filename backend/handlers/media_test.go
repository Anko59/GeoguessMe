package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v4"
)

// TestChallengeMediaViewWindow pins the challenge viewing-window contract:
// media is served while it has never been fully delivered, the window starts
// at the first full delivery, and a re-fetch after the window is always
// denied (view-once).
func TestChallengeMediaViewWindow(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-2", GroupID: "00000000-0000-0000-0000-000000000001", StorageKey: "photos/media", MIMEType: "image/png", ByteSize: 4, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	if err := store.Put(context.Background(), photo.StorageKey, bytes.NewReader([]byte("data")), 4, photo.MIMEType); err != nil {
		t.Fatal(err)
	}
	serve := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := requestWithUser(http.MethodGet, "/", "", "user-1")
		request.SetPathValue("photoID", photo.ID)
		ServeChallengeMedia(recorder, request)
		return recorder
	}
	mock := handlerMock(t)

	// A player who never received the media can still fetch it after the
	// original accept window, as long as the challenge is still live.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(nil, now.Add(-time.Minute)))
	mock.ExpectQuery("UPDATE challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), int64(RuntimeConfig.ViewWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at"}).AddRow(now.Add(RuntimeConfig.ViewWindow)))
	if recorder := serve(t); recorder.Code != http.StatusOK {
		t.Fatalf("never-delivered media = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// A re-fetch after the window has closed is denied.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(now.Add(-time.Minute), now.Add(-time.Minute)))
	if recorder := serve(t); recorder.Code != http.StatusForbidden {
		t.Fatalf("expired re-fetch = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// Within the delivered window the media is served again; the window is
	// still not extended for a second delivery.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(now.Add(-time.Minute), now.Add(time.Hour)))
	mock.ExpectQuery("UPDATE challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), int64(RuntimeConfig.ViewWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at"}).AddRow(now.Add(time.Hour)))
	if recorder := serve(t); recorder.Code != http.StatusOK {
		t.Fatalf("within-window re-fetch = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestConfirmChallengeMediaDeliveredReturnsAuthoritativeDeadline(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	photoID := "00000000-0000-0000-0000-000000000002"
	expiresAt := time.Now().UTC().Add(RuntimeConfig.ViewWindow)
	mock.ExpectQuery("UPDATE challenge_views").
		WithArgs(photoID, "user-1", pgxmock.AnyArg(), int64(RuntimeConfig.ViewWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at"}).AddRow(expiresAt))
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.SetPathValue("photoID", photoID)
	recorder := httptest.NewRecorder()
	ConfirmChallengeMediaDelivered(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("view_expires_at")) {
		t.Fatalf("delivery confirmation = %d (%s)", recorder.Code, recorder.Body.String())
	}
}
