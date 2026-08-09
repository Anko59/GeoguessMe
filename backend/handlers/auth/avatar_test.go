package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v4"
)

func newAvatarAPI(t *testing.T, mock pgxmock.PgxPoolIface, store storage.ObjectStore) *AuthAPI {
	t.Helper()
	api := newAuthAPI(t, mock, store)
	return api
}

func avatarStore(t *testing.T) *storage.LocalStore {
	t.Helper()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestUploadAvatarSuccess(t *testing.T) {
	store := avatarStore(t)
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, store)
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	updated := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(updated))
	request := avatarUploadRequest(t, mustDecodeBase64(onePixelPNG))
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
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
	api := newAvatarAPI(t, newAuthMockPool(t), avatarStore(t))
	request := avatarUploadRequest(t, []byte("not an image"))
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("non-image upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "JPG, PNG, or WebP") {
		t.Fatalf("non-image error = %s", recorder.Body.String())
	}
}

func TestUploadAvatarRejectsOversizedMultipartWithHelpfulMessage(t *testing.T) {
	api := newAvatarAPI(t, newAuthMockPool(t), avatarStore(t))
	api.cfg.AvatarMaxBytes = 2 * 1024 * 1024
	request := avatarUploadRequest(t, bytes.Repeat([]byte{'x'}, 4*1024*1024))
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "smaller than 2 MiB") {
		t.Fatalf("oversized upload error = %s", recorder.Body.String())
	}
}

func TestUploadAvatarRejectsMalformedMultipartWithHelpfulMessage(t *testing.T) {
	api := newAvatarAPI(t, newAuthMockPool(t), avatarStore(t))
	request := requestWithUser(http.MethodPost, "/", "not multipart data", "user-1")
	request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "could not read") {
		t.Fatalf("malformed upload error = %s", recorder.Body.String())
	}
}

func TestUploadAvatarRejectsMissingPhoto(t *testing.T) {
	api := newAvatarAPI(t, newAuthMockPool(t), avatarStore(t))
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
	request.ContentLength = int64(body.Len())
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing photo status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestUploadAvatarRejectsWrongMethod(t *testing.T) {
	api := newAvatarAPI(t, newAuthMockPool(t), nil)
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method check status = %d", recorder.Code)
	}
}

const avatarTestUserID = "11111111-1111-1111-1111-111111111111"

func TestServeUserAvatarSuccess(t *testing.T) {
	store := avatarStore(t)
	if err := store.Put(context.Background(), avatarStorageKey(avatarTestUserID), bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0}), 4, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, store)
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	api.ServeUserAvatar(recorder, request)
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
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, avatarStore(t))
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	api.ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("default avatar status = %d", recorder.Code)
	}
}

func TestServeUserAvatarUnknownUser(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, avatarStore(t))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnError(errors.New("no rows"))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	api.ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown user status = %d", recorder.Code)
	}
}

func TestServeUserAvatarMissingObject(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, avatarStore(t))
	user := &models.User{ID: avatarTestUserID, Username: "alice", Email: "alice@example.test", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(avatarTestUserID).WillReturnRows(handlerUserRows(user))
	recorder := httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", avatarTestUserID)
	request.SetPathValue("userID", avatarTestUserID)
	api.ServeUserAvatar(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing object status = %d", recorder.Code)
	}
}

func TestUploadAvatarStorageUnavailable(t *testing.T) {
	api := newAuthAPI(t, newAuthMockPool(t), nil)
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, requestWithUser(http.MethodPost, "/", "", "user-1"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	api.ServeUserAvatar(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil store serve status = %d", recorder.Code)
	}
}

func TestUploadAvatarDBErrorRollsBackStorage(t *testing.T) {
	store := avatarStore(t)
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, store)
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnError(errors.New("db error"))
	request := avatarUploadRequest(t, mustDecodeBase64(onePixelPNG))
	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("db error status = %d", recorder.Code)
	}
	if _, err := store.Get(context.Background(), avatarStorageKey("user-1")); err == nil {
		t.Fatal("object should have been deleted after db error")
	}
}

func TestUploadAvatarDBErrorRestoresPreviousCustomAvatar(t *testing.T) {
	store := avatarStore(t)
	key := avatarStorageKey("user-1")
	previous := []byte("previous-avatar")
	if err := store.Put(context.Background(), key, bytes.NewReader(previous), int64(len(previous)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, store)
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnError(errors.New("db error"))

	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, avatarUploadRequest(t, mustDecodeBase64(onePixelPNG)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("db error status = %d", recorder.Code)
	}
	reader, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	restored, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, previous) {
		t.Fatalf("restored avatar = %q, want %q", restored, previous)
	}
}

// largeAvatarJPEG renders an incompressible random JPEG of at least minBytes,
// reproducing a real phone photo that exceeds the former hardcoded avatar cap.
func largeAvatarJPEG(t *testing.T, minBytes int) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	for size := 2048; size <= 4608; size += 256 {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i] = byte(rng.Intn(256))
			img.Pix[i+1] = byte(rng.Intn(256))
			img.Pix[i+2] = byte(rng.Intn(256))
			img.Pix[i+3] = 255
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
			t.Fatal(err)
		}
		if buf.Len() >= minBytes {
			return buf.Bytes()
		}
	}
	t.Fatalf("could not render a JPEG >= %d bytes", minBytes)
	return nil
}

func TestUploadAvatarAcceptsLargePhoto(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAvatarAPI(t, mock, avatarStore(t))
	photo := largeAvatarJPEG(t, 10<<20)
	api.cfg.AvatarMaxBytes = int64(len(photo)) + 1<<20
	api.cfg.UploadMaxPixels = 25_000_000
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	updated := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(updated))

	recorder := httptest.NewRecorder()
	api.UploadAvatar(recorder, avatarUploadRequest(t, photo))
	if recorder.Code != http.StatusOK {
		t.Fatalf("large photo upload status = %d (%s)", recorder.Code, recorder.Body.String())
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

// onePixelPNG and mustDecodeBase64 mirror the shared test media helpers in the
// handlers package; the auth sub-package cannot import the parent test files.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg=="

func mustDecodeBase64(value string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return decoded
}
