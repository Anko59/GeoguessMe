package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg=="

func handlerPhotoRows(photo *models.Photo) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "group_id", "url", "storage_key", "mime_type", "byte_size", "lat", "long", "lifecycle_status", "created_at", "expires_at", "retention_at"}).
		AddRow(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt)
}

func multipartUpload(t *testing.T, groupID string) (*http.Request, error) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("group_id", groupID)
	_ = writer.WriteField("lat", "48.8566")
	_ = writer.WriteField("long", "2.3522")
	part, err := writer.CreateFormFile("photo", "photo.png")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(mustDecodeBase64(onePixelPNG)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request, nil
}

func mustDecodeBase64(value string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return decoded
}

func TestUploadAcceptAndServeMedia(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO photos").WithArgs(pgxmock.AnyArg(), "user-1", groupID, "", pgxmock.AnyArg(), "image/png", pgxmock.AnyArg(), 48.8566, 2.3522, "ready", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	request, err := multipartUpload(t, groupID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	UploadPhoto(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-2", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", ByteSize: 4, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	if err := store.Put(context.Background(), photo.StorageKey, bytes.NewReader([]byte("data")), 4, photo.MIMEType); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT photo_id, user_id").WithArgs(photo.ID, "user-1").WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	acceptRequest := requestWithUser(http.MethodPost, "/", "", "user-1")
	acceptRequest.SetPathValue("photoID", photo.ID)
	AcceptChallenge(recorder, acceptRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("accept status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT view_expires_at").WithArgs(photo.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"view_expires_at"}).AddRow(now.Add(time.Hour)))
	recorder = httptest.NewRecorder()
	mediaRequest := requestWithUser(http.MethodGet, "/", "", "user-1")
	mediaRequest.SetPathValue("photoID", photo.ID)
	ServeChallengeMedia(recorder, mediaRequest)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "data" {
		t.Fatalf("media response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestChallengeResultsAndChatRejection(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-1", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at", "username", "avatar"}).AddRow("guess-1", photo.ID, "user-2", groupID, 48.8, 2.3, 80, 10.0, now, "bob", "b.png"))
	recorder := httptest.NewRecorder()
	resultsRequest := requestWithUser(http.MethodGet, "/", "", "user-1")
	resultsRequest.SetPathValue("photoID", photo.ID)
	GetChallengeResults(recorder, resultsRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("results status = %d", recorder.Code)
	}

	RuntimeConfig.AllowedOrigins = []string{"http://allowed.test"}
	HubInstance = chat.NewHub(nil)
	badOrigin := requestWithUser(http.MethodGet, "/?group_id="+groupID+"&ticket=t", "", "user-1")
	badOrigin.Header.Set("Origin", "http://evil.test")
	recorder = httptest.NewRecorder()
	HandleChat(recorder, badOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", recorder.Code)
	}
	HubInstance = nil
}

type failingStore struct{ err error }

func (s failingStore) Put(context.Context, string, io.Reader, int64, string) error { return s.err }
func (s failingStore) Delete(context.Context, string) error                        { return nil }
func (s failingStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, storage.ErrObjectNotFound
}
func (s failingStore) Stat(context.Context, string) (int64, error) {
	return 0, storage.ErrObjectNotFound
}
func (s failingStore) Health(context.Context) error { return s.err }

func TestUploadStorageFailureAndChallengeErrors(t *testing.T) {
	setupHandlers(t)
	MediaStore = failingStore{err: errors.New("storage down")}
	mock := handlerMock(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("00000000-0000-0000-0000-000000000001", "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	request, err := multipartUpload(t, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	UploadPhoto(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("storage failure status = %d", recorder.Code)
	}
	for _, expected := range []error{repository.ErrForbidden, repository.ErrOwnPhoto, repository.ErrNotFound, repository.ErrChallengeExpired, errors.New("other")} {
		recorder = httptest.NewRecorder()
		challengeError(recorder, expected)
		if recorder.Code == http.StatusOK {
			t.Fatalf("challenge error %v returned success", expected)
		}
	}
}
