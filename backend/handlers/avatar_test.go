package handlers

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	updated := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
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
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
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

func TestUploadAvatarDBErrorRestoresPreviousCustomAvatar(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	key := avatarStorageKey("user-1")
	previous := []byte("previous-avatar")
	if err := store.Put(context.Background(), key, bytes.NewReader(previous), int64(len(previous)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mock := handlerMock(t)
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnError(errors.New("db error"))

	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, avatarUploadRequest(t, mustDecodeBase64(onePixelPNG)))
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
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	// A high-resolution phone photo is larger than the shared 10 MiB media cap.
	// The avatar-specific limit must accept it and normalize it down to a small
	// thumbnail without changing challenge upload limits.
	photo := largeAvatarJPEG(t, 10<<20)
	RuntimeConfig.AvatarMaxBytes = int64(len(photo)) + 1<<20
	RuntimeConfig.UploadMaxPixels = 25_000_000
	mock := handlerMock(t)
	current := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	updated := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "custom"}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(current))
	mock.ExpectExec("UPDATE users SET avatar").WithArgs("custom", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnRows(handlerUserRows(updated))

	recorder := httptest.NewRecorder()
	UploadAvatar(recorder, avatarUploadRequest(t, photo))
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

func groupPhotoUploadRequest(t *testing.T, groupID string, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("group_id", groupID); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("photo", "group.png")
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

func TestGroupNotificationSettings(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	groupID := "00000000-0000-0000-0000-000000000001"
	member := func() {
		mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	}
	member()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"enabled"}).AddRow(true))
	requireStatus(t, GroupNotifications, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusOK)
	member()
	mock.ExpectExec("INSERT INTO group_notification_preferences").WithArgs(groupID, "user-1", false).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	requireStatus(t, GroupNotifications, requestWithUser(http.MethodPut, "/?group_id="+groupID, `{"enabled":false}`, "user-1"), http.StatusOK)
	member()
	requireStatus(t, GroupNotifications, requestWithUser(http.MethodDelete, "/?group_id="+groupID, "", "user-1"), http.StatusMethodNotAllowed)
}

func TestUploadAndServeGroupPhoto(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").WithArgs(groupID).
		WillReturnRows(pgxmock.NewRows([]string{"storage_key"}))
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(groupID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	requireStatus(t, GroupPhoto, groupPhotoUploadRequest(t, groupID, mustDecodeBase64(onePixelPNG)), http.StatusOK)

	const key = "groups/group-1/photo/current"
	if err := store.Put(context.Background(), key, bytes.NewReader([]byte("photo")), 5, "image/png"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").WithArgs(groupID).
		WillReturnRows(pgxmock.NewRows([]string{"group_id", "storage_key", "mime_type", "byte_size", "created_at"}).
			AddRow(groupID, key, "image/png", 5, time.Now()))
	recorder := httptest.NewRecorder()
	GroupPhoto(recorder, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "photo" ||
		recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("group photo response = %d %q, cache %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Cache-Control"))
	}
	requireStatus(t, GroupPhoto, requestWithUser(http.MethodDelete, "/", "", "user-1"), http.StatusMethodNotAllowed)
}

func TestReactionAndGroupSettingFailures(t *testing.T) {
	setupHandlers(t)
	messageID := "00000000-0000-0000-0000-000000000002"
	groupID := "00000000-0000-0000-0000-000000000001"
	reactionRequest := func(method, emoji string) *http.Request {
		request := requestWithUser(method, "/", `{"emoji":"`+emoji+`"}`, "user-1")
		request.SetPathValue("messageID", messageID)
		return request
	}
	requireStatus(t, SetMessageReaction, reactionRequest(http.MethodPost, "👍"), http.StatusMethodNotAllowed)
	requireStatus(t, SetMessageReaction, reactionRequest(http.MethodPut, "👎"), http.StatusBadRequest)

	mock := handlerMock(t)
	columns := []string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}
	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).
		WillReturnRows(pgxmock.NewRows(columns))
	requireStatus(t, SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusNotFound)

	messageRows := func(kind string) *pgxmock.Rows {
		return pgxmock.NewRows(columns).
			AddRow(messageID, groupID, "user-2", "bob", "", kind, nil, nil, nil, nil, "hello", time.Now())
	}
	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows("system"))
	mock.ExpectQuery("SELECT message_id, emoji, COUNT").WithArgs([]string{messageID}, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "emoji", "count", "reacted"}))
	requireStatus(t, SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusBadRequest)

	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows("text"))
	mock.ExpectQuery("SELECT message_id, emoji, COUNT").WithArgs([]string{messageID}, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "emoji", "count", "reacted"}))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusForbidden)

	requireStatus(t, GroupNotifications, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, GroupNotifications, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusForbidden)
}

func TestGroupPhotoAvailabilityFailures(t *testing.T) {
	setupHandlers(t)
	groupID := "00000000-0000-0000-0000-000000000001"
	requireStatus(t, GroupPhoto, groupPhotoUploadRequest(t, groupID, mustDecodeBase64(onePixelPNG)), http.StatusServiceUnavailable)
	requireStatus(t, GroupPhoto, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusServiceUnavailable)

	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").WithArgs(groupID).
		WillReturnError(errors.New("database unavailable"))
	requireStatus(t, GroupPhoto, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusNotFound)
}
