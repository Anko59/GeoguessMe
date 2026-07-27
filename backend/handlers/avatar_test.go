package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v4"
)

func TestUploadAvatarSuccess(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	updated := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(updated))
	request := avatarUploadRequest(t, mustDecodeBase64(onePixelPNG))
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	// Verify the object was stored.
	reader, err := store.Get(context.Background(), avatarStorageKey("user-1"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || len(data) == 0 {
		t.Fatal("stored avatar is empty")
	}
}

func TestUploadAvatarRejectsNonImage(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	request := avatarUploadRequest(t, []byte("not an image"))
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("non-image upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestUploadAvatarRejectsMissingPhoto(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("not_photo", "ignored"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing photo status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestUploadAvatarRejectsWrongMethod(t *testing.T) {
	setupHandlers(t)
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method check status = %d", recorder.Code)
	}
}

const avatarTestUserID = "11111111-1111-1111-1111-111111111111"

func TestServeUserAvatarSuccess(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	// Store a small JPEG as if a custom avatar was uploaded.
	if err := store.Put(context.Background(), avatarStorageKey(avatarTestUserID), bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0}), 4, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mock := handlerMock(t)
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("serve status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q", got)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("body is empty")
	}
}

func TestServeUserAvatarDefaultUser(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("default avatar status = %d", recorder.Code)
	}
}

func TestServeUserAvatarUnknownUser(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnError(errors.New("no rows"))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown user status = %d", recorder.Code)
	}
}

func TestServeUserAvatarMissingObject(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing object status = %d", recorder.Code)
	}
}

func TestUploadAvatarStorageUnavailable(t *testing.T) {
	setupHandlers(t)
	MediaStore = nil
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, requestWithUser(http.MethodPost, "/", "", "user-1"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	ServeUserAvatar(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store serve status = %d", recorder.Code)
	}
}

func TestUploadAvatarDBErrorRollsBackStorage(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnError(errors.New("db error"))
	request := avatarUploadRequest(t, mustDecodeBase64(onePixelPNG))
	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("db error status = %d", recorder.Code)
	}
	// Object should have been cleaned up.
	if _, err := store.Get(context.Background(), avatarStorageKey("user-1")); err == nil {
		t.Fatal("object should have been deleted after db error")
	}
}

func avatarUploadRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("photo", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
