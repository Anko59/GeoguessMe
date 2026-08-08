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
	"strings"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg=="

func handlerPhotoRows(photo *models.Photo) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "group_id", "url", "storage_key", "mime_type", "byte_size", "lat", "long", "lifecycle_status", "hide_location", "created_at", "expires_at", "retention_at"}).
		AddRow(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt)
}

func multipartUpload(t *testing.T, groupID string) (*http.Request, error) {
	return multipartMediaUpload(t, groupID, "photo.png", mustDecodeBase64(onePixelPNG))
}

func multipartMediaUpload(t *testing.T, groupID, filename string, payload []byte) (*http.Request, error) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("group_id", groupID)
	_ = writer.WriteField("lat", "48.8566")
	_ = writer.WriteField("long", "2.3522")
	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(payload); err != nil {
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

func multipartChatMediaUpload(t *testing.T, groupID, filename string, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("group_id", groupID); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("content", "look at this"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("media", filename)
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

func mustDecodeBase64(value string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return decoded
}

func TestUploadAcceptAndServeMedia(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	countedStore := &countingStore{ObjectStore: store}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, countedStore, handlerConfig(), nil, nil, time.Now)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO photos").WithArgs(pgxmock.AnyArg(), "user-1", groupID, "", pgxmock.AnyArg(), "image/png", pgxmock.AnyArg(), 48.8566, 2.3522, "ready", false, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	request, err := multipartUpload(t, groupID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, request)
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
	gameAPI.AcceptChallenge(recorder, acceptRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("accept status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(nil, now.Add(time.Hour)))
	mock.ExpectQuery("UPDATE challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), int64(handlerConfig().ViewWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at"}).AddRow(now.Add(handlerConfig().ViewWindow)))
	recorder = httptest.NewRecorder()
	mediaRequest := requestWithUser(http.MethodGet, "/", "", "user-1")
	mediaRequest.SetPathValue("photoID", photo.ID)
	gameAPI.ServeChallengeMedia(recorder, mediaRequest)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "data" {
		t.Fatalf("media response = %d %q", recorder.Code, recorder.Body.String())
	}
	if countedStore.getCalls != 1 || countedStore.statCalls != 0 {
		t.Fatalf("media storage calls = get:%d stat:%d, want get:1 stat:0", countedStore.getCalls, countedStore.statCalls)
	}
}

func TestUploadRecordedVideo(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO photos").WithArgs(pgxmock.AnyArg(), "user-1", groupID, "", pgxmock.AnyArg(), "video/webm", pgxmock.AnyArg(), 48.8566, 2.3522, "ready", false, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	webm := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}
	request, err := multipartMediaUpload(t, groupID, "capture.webm", webm)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("video upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestSetAndRemoveMessageReaction(t *testing.T) {
	mock := newMockPool(t)
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	chatAPI := newChatAPI(t, mock, nil, hub)
	now := time.Now().UTC().Truncate(time.Microsecond)
	messageID := "00000000-0000-0000-0000-000000000002"
	messageRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}).
			AddRow(messageID, "group-1", "user-2", "bob", "", "text", nil, nil, nil, nil, "hello", now)
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows())
		mock.ExpectQuery("SELECT message_id, reaction, COUNT").WithArgs([]string{messageID}, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
		mock.ExpectQuery("SELECT EXISTS").WithArgs("group-1", "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		if method == http.MethodPut {
			mock.ExpectExec("INSERT INTO message_reactions").WithArgs(messageID, "user-1", "like").
				WillReturnResult(pgxmock.NewResult("INSERT", 1))
		} else {
			mock.ExpectExec("DELETE FROM message_reactions").WithArgs(messageID, "user-1", "like").
				WillReturnResult(pgxmock.NewResult("DELETE", 1))
		}
		mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows())
		mock.ExpectQuery("SELECT message_id, reaction, COUNT").WithArgs([]string{messageID}, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
		request := requestWithUser(method, "/", `{"reaction":"like"}`, "user-1")
		request.SetPathValue("messageID", messageID)
		requireStatus(t, chatAPI.SetMessageReaction, request, http.StatusOK)
	}
}

func TestUploadAndServeChatMedia(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock := newMockPool(t)
	chatAPI := newChatAPI(t, mock, store, hub)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"))
	mock.ExpectExec("INSERT INTO chat_media").WithArgs(pgxmock.AnyArg(), groupID, "user-1", pgxmock.AnyArg(), "image/png", pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO messages").WithArgs(pgxmock.AnyArg(), groupID, "user-1", pgxmock.AnyArg(), pgxmock.AnyArg(), "look at this", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	chatAPI.UploadChatMedia(recorder, multipartChatMediaUpload(t, groupID, "chat.png", mustDecodeBase64(onePixelPNG)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("chat upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	assetID := "00000000-0000-0000-0000-000000000002"
	asset := &models.ChatMedia{ID: assetID, GroupID: groupID, UserID: "user-2", StorageKey: "chat-media/known", MIMEType: "video/webm", ByteSize: 4, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	if err := store.Put(context.Background(), asset.StorageKey, bytes.NewReader([]byte("data")), asset.ByteSize, asset.MIMEType); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(assetID).WillReturnRows(
		pgxmock.NewRows([]string{"group_id", "user_id", "storage_key", "mime_type", "byte_size", "created_at"}).AddRow(asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt),
	)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	recorder = httptest.NewRecorder()
	request := requestWithUser(http.MethodGet, "/", "", "user-1")
	request.SetPathValue("mediaID", assetID)
	chatAPI.ServeChatMedia(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "data" {
		t.Fatalf("chat media response = %d %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("chat media cache policy = %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "video/webm" {
		t.Fatalf("chat media type = %q", got)
	}

	requireStatus(t, chatAPI.UploadChatMedia, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusMethodNotAllowed)
	requireStatus(t, chatAPI.ServeChatMedia, requestWithUser(http.MethodPost, "/", "", "user-1"), http.StatusMethodNotAllowed)
}

func TestChatMediaFailureResponses(t *testing.T) {
	groupID := "00000000-0000-0000-0000-000000000001"
	payload := mustDecodeBase64(onePixelPNG)
	mock := newMockPool(t)
	// A ChatAPI without an object store reports chat media unavailable before
	// touching any persistence.
	nilStoreAPI := NewChatAPI(chatrepo.NewRepository(mock), repository.NewRepository(mock).Groups, nil, handlerConfig(), nil, time.Now, repository.NewRepository(mock))
	requireStatus(t, nilStoreAPI.UploadChatMedia, multipartChatMediaUpload(t, groupID, "chat.png", payload), http.StatusServiceUnavailable)

	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	chatAPI := newChatAPI(t, mock, store, hub)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, chatAPI.UploadChatMedia, multipartChatMediaUpload(t, groupID, "chat.png", payload), http.StatusForbidden)

	mediaID := "00000000-0000-0000-0000-000000000002"
	serve := func() *http.Request {
		request := requestWithUser(http.MethodGet, "/", "", "user-1")
		request.SetPathValue("mediaID", mediaID)
		return request
	}
	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(mediaID).WillReturnError(pgx.ErrNoRows)
	requireStatus(t, chatAPI.ServeChatMedia, serve(), http.StatusNotFound)

	asset := &models.ChatMedia{ID: mediaID, GroupID: groupID, UserID: "user-2", StorageKey: "chat-media/missing", MIMEType: "image/png", ByteSize: 4, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	assetRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"group_id", "user_id", "storage_key", "mime_type", "byte_size", "created_at"}).AddRow(asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt)
	}
	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(mediaID).WillReturnRows(assetRows())
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, chatAPI.ServeChatMedia, serve(), http.StatusForbidden)

	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(mediaID).WillReturnRows(assetRows())
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	requireStatus(t, chatAPI.ServeChatMedia, serve(), http.StatusGone)
}

type countingStore struct {
	storage.ObjectStore
	getCalls  int
	statCalls int
}

func (s *countingStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	s.getCalls++
	return s.ObjectStore.Get(ctx, key)
}

func (s *countingStore) Stat(ctx context.Context, key string) (int64, error) {
	s.statCalls++
	return s.ObjectStore.Stat(ctx, key)
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
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, failingStore{err: errors.New("storage down")}, handlerConfig(), nil, nil, time.Now)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("00000000-0000-0000-0000-000000000001", "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	request, err := multipartUpload(t, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("storage failure status = %d", recorder.Code)
	}
	for _, expected := range []error{groups.ErrForbidden, groups.ErrOwnPhoto, groups.ErrNotFound, groups.ErrChallengeExpired, errors.New("other")} {
		recorder = httptest.NewRecorder()
		challengeError(recorder, expected)
		if recorder.Code == http.StatusOK {
			t.Fatalf("challenge error %v returned success", expected)
		}
	}
}

func TestHandleInvitePreview(t *testing.T) {
	cfg := handlerConfig()
	cfg.PublicURL = "https://geoguessme.com"
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, nil, cfg, nil, nil, time.Now)
	now := time.Now().UTC()
	group := &models.Group{ID: "00000000-0000-0000-0000-000000000001", Name: "Paris", Code: "ABC123", CreatedAt: now}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/invite/ABC123?from=Alice", nil)
	req.SetPathValue("code", "ABC123")
	gameAPI.HandleInvitePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("invite preview status = %d", rec.Code)
	}
	body := rec.Body.String()
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	mustContain := []string{"og:title", "og:description", "Join Paris on GeoGuessMe", "Alice invites you", "og:image", "https://geoguessme.com/logo.png", "/group/join?code=ABC123"}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestHandleInvitePreviewWithoutInviter(t *testing.T) {
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, nil, handlerConfig(), nil, nil, time.Now)
	group := &models.Group{ID: "00000000-0000-0000-0000-000000000001", Name: "Paris", Code: "DEF456", CreatedAt: time.Now().UTC()}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/invite/DEF456", nil)
	req.SetPathValue("code", "DEF456")
	gameAPI.HandleInvitePreview(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("invite preview status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Join the group Paris on GeoGuessMe!") {
		t.Fatalf("expected fallback message, got %s", rec.Body.String())
	}
}
