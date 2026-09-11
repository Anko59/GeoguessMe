package feed

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"geoguessme/handlers"
	"geoguessme/internal/config"

	"github.com/pashagolub/pgxmock/v4"
)

func publicUpload(t *testing.T, caption, lat string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("photo", "place.png")
	if err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(png); err != nil {
		t.Fatal(err)
	}
	for field, value := range map[string]string{"caption": caption, "lat": lat, "long": "2.3"} {
		if err := form.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/feed/challenges", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	return r.WithContext(handlers.WithUserID(r.Context(), "viewer"))
}

type deletionRecorder struct {
	keys       []string
	contextErr error
}

func (d *deletionRecorder) EnqueueMediaDeletion(ctx context.Context, _ string, keys []string) error {
	d.keys = append(d.keys, keys...)
	d.contextErr = ctx.Err()
	return nil
}

func TestUploadCreatesOnlyExplicitPublicPhotoAndCompensatesFailures(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		storageErr, databaseErr, deleteErr error
		status                             int
	}{
		{"published", nil, nil, nil, 201},
		{"storage unavailable", errors.New("store offline"), nil, nil, 502},
		{"database failure", nil, errors.New("database offline"), nil, 500},
		{"durable deletion fallback", nil, errors.New("database offline"), errors.New("store offline"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, mock := mockAPI(t)
			store := &fakeStore{putErr: tc.storageErr, deleteErr: tc.deleteErr}
			deletions := &deletionRecorder{}
			a.store = store
			a.deletions = deletions
			a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
			if tc.storageErr == nil {
				insert := mock.ExpectExec("INSERT INTO public_challenges").WithArgs(pgxmock.AnyArg(), "viewer", "A place", pgxmock.AnyArg(), "image/png", pgxmock.AnyArg(), 48.8, 2.3, pgxmock.AnyArg())
				if tc.databaseErr != nil {
					insert.WillReturnError(tc.databaseErr)
				} else {
					insert.WillReturnResult(pgxmock.NewResult("INSERT", 1))
				}
			}
			w := httptest.NewRecorder()
			a.Upload(w, publicUpload(t, " A place ", "48.8"))
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if tc.status == 201 && (len(store.deleted) != 0 || strings.Contains(w.Body.String(), "storage_key")) {
				t.Fatal("published photo was removed or its key exposed")
			}
			if tc.status != 201 && len(store.deleted) != 1 {
				t.Fatalf("cleanup attempts %v", store.deleted)
			}
			if tc.deleteErr != nil && (len(deletions.keys) != 1 || deletions.contextErr != nil) {
				t.Fatalf("durable deletion: %+v", deletions)
			}
		})
	}
}

func TestPublicUploadRejectsCaptionAndCoordinatesBeforeStorage(t *testing.T) {
	for _, tc := range []struct{ caption, lat string }{{strings.Repeat("a", 501), "48.8"}, {"", "NaN"}, {"", "91"}, {"", ""}} {
		a, _ := mockAPI(t)
		store := &fakeStore{}
		a.store = store
		a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
		w := httptest.NewRecorder()
		a.Upload(w, publicUpload(t, tc.caption, tc.lat))
		if w.Code != 400 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
}

func TestCompensationSurvivesCancelledRequest(t *testing.T) {
	a, _ := mockAPI(t)
	a.store = &fakeStore{deleteErr: errors.New("unavailable")}
	deletions := &deletionRecorder{}
	a.deletions = deletions
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.compensate(ctx, "public-challenges/orphan")
	if len(deletions.keys) != 1 || deletions.contextErr != nil {
		t.Fatalf("cleanup depended on cancelled request: %+v", deletions)
	}
}

type timeoutStore struct{ fakeStore }

func (*timeoutStore) Delete(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestCompensationQueuesAfterStorageTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, _ := mockAPI(t)
		a.store = &timeoutStore{}
		deletions := &deletionRecorder{}
		a.deletions = deletions
		a.compensate(t.Context(), "public-challenges/timed-out")
		if len(deletions.keys) != 1 || deletions.contextErr != nil {
			t.Fatalf("storage timeout cancelled durable cleanup: %+v", deletions)
		}
	})
}
