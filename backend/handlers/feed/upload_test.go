package feed

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"geoguessme/handlers"
	"geoguessme/internal/config"
	"geoguessme/internal/models"
	feedrepo "geoguessme/internal/repository/feed"

	"github.com/pashagolub/pgxmock/v5"
)

func publicUpload(t *testing.T, caption, lat string) *http.Request {
	return uploadWithAudience(t, caption, lat, "", nil)
}

func uploadWithAudience(t *testing.T, caption, lat, audience string, groupIDs []string) *http.Request {
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
	if audience != "" {
		if err := form.WriteField("audience", audience); err != nil {
			t.Fatal(err)
		}
	}
	for _, groupID := range groupIDs {
		if err := form.WriteField("group_id", groupID); err != nil {
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

func TestPublicUploadValidatesAudienceAndSelectedGroups(t *testing.T) {
	for _, tc := range []struct {
		name, audience string
		groups         []string
	}{
		{name: "unknown audience", audience: "neighbors"},
		{name: "invalid group", audience: "friends", groups: []string{"not-a-uuid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := mockAPI(t)
			a.store = &fakeStore{}
			a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
			w := httptest.NewRecorder()
			a.Upload(w, uploadWithAudience(t, "A place", "48.8", tc.audience, tc.groups))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
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
				insert := mock.ExpectExec("INSERT INTO public_challenges").WithArgs(pgxmock.AnyArg(), "viewer", "A place", "public", pgxmock.AnyArg(), "image/png", pgxmock.AnyArg(), 48.8, 2.3, pgxmock.AnyArg())
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

type fakeChallengePublisher struct {
	existing    bool
	existingErr error
	createErr   error
	challenge   feedrepo.NewChallenge
	photos      []*models.Photo
	createCalls int
}

func (p *fakeChallengePublisher) ExistingFeedChallenge(context.Context, string, string) (bool, error) {
	return p.existing, p.existingErr
}

func (p *fakeChallengePublisher) CreateFeedChallenge(_ context.Context, challenge feedrepo.NewChallenge, photos []*models.Photo) (bool, error) {
	p.createCalls++
	p.challenge = challenge
	p.photos = photos
	return false, p.createErr
}

func TestUploadPublishesEverySelectedDestinationThroughTheAtomicPublisher(t *testing.T) {
	for _, tc := range []struct {
		name, audience string
		groups         []string
	}{
		{name: "public only", audience: "public"},
		{name: "friends only", audience: "friends"},
		{name: "public and groups", audience: "public", groups: []string{testID, "00000000-0000-0000-0000-000000000002"}},
		{name: "friends and groups", audience: "friends", groups: []string{testID, "00000000-0000-0000-0000-000000000002"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := mockAPI(t)
			store := &fakeStore{}
			publisher := &fakeChallengePublisher{}
			a.store = store
			a.publisher = publisher
			a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
			w := httptest.NewRecorder()
			a.Upload(w, uploadWithAudience(t, "A place", "48.8", tc.audience, tc.groups))
			if w.Code != http.StatusCreated {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if publisher.createCalls != 1 || publisher.challenge.Audience != tc.audience {
				t.Fatalf("publisher call: %+v", publisher)
			}
			if len(publisher.challenge.GroupIDs) != len(tc.groups) || len(publisher.photos) != len(tc.groups) {
				t.Fatalf("destinations: challenge=%+v photos=%+v", publisher.challenge.GroupIDs, publisher.photos)
			}
			if len(store.puts) != len(tc.groups)+1 {
				t.Fatalf("stored keys: %v", store.puts)
			}
		})
	}
}

func TestUploadIdempotencySkipsStorageAndCreationOnRetry(t *testing.T) {
	a, _ := mockAPI(t)
	store := &fakeStore{}
	publisher := &fakeChallengePublisher{existing: true}
	a.store = store
	a.publisher = publisher
	a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
	w := httptest.NewRecorder()
	a.Upload(w, uploadWithAudience(t, "A place", "48.8", "public", []string{testID}))
	if w.Code != http.StatusOK || len(store.puts) != 0 || publisher.createCalls != 0 {
		t.Fatalf("retry status=%d puts=%v creates=%d", w.Code, store.puts, publisher.createCalls)
	}
}

type failOnPutStore struct {
	fakeStore
	attempt int
	failAt  int
}

func (s *failOnPutStore) Put(ctx context.Context, key string, reader io.Reader, size int64, mime string) error {
	s.attempt++
	if s.attempt == s.failAt {
		s.puts = append(s.puts, key)
		return errors.New("store offline")
	}
	return s.fakeStore.Put(ctx, key, reader, size, mime)
}

func TestUploadCompensatesAllAttemptedDestinationsAfterPartialStorageFailure(t *testing.T) {
	a, _ := mockAPI(t)
	store := &failOnPutStore{failAt: 2}
	deletions := &deletionRecorder{}
	a.store = store
	a.deletions = deletions
	a.cfg = &config.Config{UploadMaxBytes: 1024 * 1024, UploadMaxPixels: 1000}
	w := httptest.NewRecorder()
	a.Upload(w, uploadWithAudience(t, "A place", "48.8", "public", []string{testID, "00000000-0000-0000-0000-000000000002"}))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(store.deleted) != 2 || len(deletions.keys) != 0 {
		t.Fatalf("cleanup deleted=%v queued=%v", store.deleted, deletions.keys)
	}
}

func TestPublicUploadRejectsCaptionAndCoordinatesBeforeStorage(t *testing.T) {
	for _, tc := range []struct{ caption, lat string }{{strings.Repeat("a", 281), "48.8"}, {"", "NaN"}, {"", "91"}, {"", ""}} {
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
