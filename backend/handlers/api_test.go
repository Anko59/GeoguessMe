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
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/pashagolub/pgxmock/v4"
)

func handlerConfig() *config.Config {
	return &config.Config{
		Environment: "test", PublicURL: "http://localhost:8080", JWTSecret: "test_secret_key_at_least_32_characters_long",
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, VerificationTTL: 24 * time.Hour, ResetTTL: time.Hour,
		PasswordHashCost: 4, UploadMaxBytes: 5 * 1024 * 1024, AvatarMaxBytes: 25 * 1024 * 1024, UploadMaxPixels: 100000, ChallengeTTL: time.Hour,
		ViewWindow: time.Minute, LocationHide: 48 * time.Hour, PhotoRetention: 24 * time.Hour, AllowedOrigins: []string{"http://localhost:8080"},
	}
}

// newMockPool returns an isolated mock pool for one handler test. Expectations
// are verified at cleanup, so a query that leaks across tests (for example
// through a shared package global) fails the test. No global is swapped.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

// newChatAPI builds the migrated chat transport on the caller's mock pool
// (the same pool serves membership checks and chat persistence), with the
// given store and hub. It replaces the package globals the old chat handlers
// read: MediaStore, RuntimeConfig, and HubInstance.
func newChatAPI(t *testing.T, mock pgxmock.PgxPoolIface, store storage.ObjectStore, hub *chat.Hub) *ChatAPI {
	t.Helper()
	repos := repository.NewRepository(mock)
	return NewChatAPI(repos.Chat, repos.Groups, store, handlerConfig(), hub, time.Now, repos)
}

// newGameAPI builds the migrated gameplay transport on the caller's mock pool
// (the same pool serves membership checks and gameplay persistence), with a
// temp-dir store and the test configuration. It replaces the package globals
// the old gameplay handlers read: MediaStore, RuntimeConfig, and Push.
func newGameAPI(t *testing.T, mock pgxmock.PgxPoolIface) *GameAPI {
	t.Helper()
	repos := repository.NewRepository(mock)
	return NewGameAPI(repos.Groups, repos.Chat, repos, mustTestStore(t), handlerConfig(), nil, nil, time.Now)
}

func mustTestStore(t *testing.T) storage.ObjectStore {
	t.Helper()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// validationStore is a no-op media store so handler tests can exercise the
// upload path without a real object store.
type validationStore struct{}

func (validationStore) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (validationStore) Delete(context.Context, string) error                        { return nil }
func (validationStore) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }
func (validationStore) Stat(context.Context, string) (int64, error)                 { return 0, nil }
func (validationStore) Health(context.Context) error                                { return nil }

func requestWithUser(method, target, body, userID string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	return request.WithContext(context.WithValue(request.Context(), userIDKey, userID))
}

func requireStatus(t *testing.T, handler http.HandlerFunc, request *http.Request, status int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != status {
		t.Fatalf("%s %s status = %d, want %d (%s)", request.Method, request.URL.Path, recorder.Code, status, recorder.Body.String())
	}
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

func TestGroupAndReadHandlers(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()
	group := &models.Group{ID: "00000000-0000-0000-0000-000000000001", Name: "Paris", Code: "ABC123", CreatedAt: now}
	ownerRequest := func(method, target, body string) *http.Request {
		return requestWithUser(method, target, body, "user-1")
	}

	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO groups").WithArgs(pgxmock.AnyArg(), "Created", pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO group_members").WithArgs(pgxmock.AnyArg(), "user-1", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	gameAPI.CreateGroup(recorder, ownerRequest(http.MethodPost, "/", `{"name":"Created"}`))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create group status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs("ABC123").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-1", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder = httptest.NewRecorder()
	gameAPI.JoinGroup(recorder, ownerRequest(http.MethodPost, "/", `{"code":"abc123"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("join group status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE id").WithArgs(group.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	recorder = httptest.NewRecorder()
	gameAPI.GetGroupDetails(recorder, ownerRequest(http.MethodGet, "/?id="+group.ID, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("group details status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT u.id, u.username, u.avatar").WithArgs(group.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar"}).AddRow("user-1", "alice", "avatar.png"))
	recorder = httptest.NewRecorder()
	gameAPI.GetGroupMembers(recorder, ownerRequest(http.MethodGet, "/?id="+group.ID, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("members status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT u.id, u.username, u.avatar").WithArgs(group.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "score", "count", "average", "total_points"}).AddRow("user-1", "alice", "avatar.png", 10, 1, 10.0, 10))
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND g\.group_id = \$1`).WithArgs(group.ID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}))
	recorder = httptest.NewRecorder()
	gameAPI.GetLeaderboard(recorder, ownerRequest(http.MethodGet, "/?group_id="+group.ID, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("leaderboard status = %d", recorder.Code)
	}

	groupsAPI := NewGroupAPI(stubGroupReader{groups: []models.Group{*group}})
	recorder = httptest.NewRecorder()
	groupsAPI.GetUserGroups(recorder, ownerRequest(http.MethodGet, "/", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("user groups status = %d", recorder.Code)
	}

	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT .*FROM messages.*ORDER BY m.created_at DESC").WithArgs(group.ID, 500).WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}))
	recorder = httptest.NewRecorder()
	chatAPI.GetGroupMessages(recorder, ownerRequest(http.MethodGet, "/?group_id="+group.ID, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("messages status = %d", recorder.Code)
	}
}

func TestMessagesIgnoreLegacyAfterID(t *testing.T) {
	// The legacy after_id query parameter was removed (PR 12): a request that
	// carries after_id without a cursor is served from the latest page. The
	// parameter is no longer honored as a positioning mechanism, so no
	// message-id resolution query may run — the only expected query is the
	// latest-page fetch. An unexpected CursorAfterMessage resolution would
	// fail the mock.
	mock := newMockPool(t)
	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT .*FROM messages.*ORDER BY m.created_at DESC").WithArgs(groupID, 500).
		WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}))
	requireStatus(t, chatAPI.GetGroupMessages, requestWithUser(http.MethodGet, "/?group_id="+groupID+"&after_id=message-1", "", "user-1"), http.StatusOK)
}

func TestTicketAndUnauthorizedMiddlewareBranches(t *testing.T) {
	mock := newMockPool(t)
	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO websocket_tickets").WithArgs(pgxmock.AnyArg(), "user-1", groupID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	chatAPI.CreateWebSocketTicket(recorder, requestWithUser(http.MethodPost, "/?group_id="+groupID, "", "user-1"))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("ticket status = %d", recorder.Code)
	}

	// A nil hub makes the WebSocket endpoint report chat unavailable. The hub
	// is an injected dependency, not a swappable global.
	requireStatus(t, newChatAPI(t, mock, mustTestStore(t), nil).HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
}

func TestGroupNotificationSettings(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	groupID := "00000000-0000-0000-0000-000000000001"
	member := func() {
		mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	}
	member()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"enabled"}).AddRow(true))
	requireStatus(t, gameAPI.GroupNotifications, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusOK)
	member()
	mock.ExpectExec("INSERT INTO group_notification_preferences").WithArgs(groupID, "user-1", false).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	requireStatus(t, gameAPI.GroupNotifications, requestWithUser(http.MethodPut, "/?group_id="+groupID, `{"enabled":false}`, "user-1"), http.StatusOK)
	member()
	requireStatus(t, gameAPI.GroupNotifications, requestWithUser(http.MethodDelete, "/?group_id="+groupID, "", "user-1"), http.StatusMethodNotAllowed)
}

func TestUploadAndServeGroupPhoto(t *testing.T) {
	store := mustTestStore(t)
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
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
	requireStatus(t, gameAPI.GroupPhoto, groupPhotoUploadRequest(t, groupID, mustDecodeBase64(onePixelPNG)), http.StatusOK)

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
	gameAPI.GroupPhoto(recorder, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "photo" ||
		recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("group photo response = %d %q, cache %q", recorder.Code, recorder.Body.String(), recorder.Header().Get("Cache-Control"))
	}
	requireStatus(t, gameAPI.GroupPhoto, requestWithUser(http.MethodDelete, "/", "", "user-1"), http.StatusMethodNotAllowed)
}

func TestReactionAndGroupSettingFailures(t *testing.T) {
	messageID := "00000000-0000-0000-0000-000000000002"
	groupID := "00000000-0000-0000-0000-000000000001"
	reactionRequest := func(method, reaction string) *http.Request {
		request := requestWithUser(method, "/", `{"reaction":"`+reaction+`"}`, "user-1")
		request.SetPathValue("messageID", messageID)
		return request
	}
	mock := newMockPool(t)
	chatAPI := newChatAPI(t, mock, nil, nil)
	gameAPI := newGameAPI(t, mock)
	requireStatus(t, chatAPI.SetMessageReaction, reactionRequest(http.MethodPost, "👍"), http.StatusMethodNotAllowed)
	requireStatus(t, chatAPI.SetMessageReaction, reactionRequest(http.MethodPut, "👎"), http.StatusBadRequest)

	columns := []string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}
	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).
		WillReturnRows(pgxmock.NewRows(columns))
	requireStatus(t, chatAPI.SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusNotFound)

	messageRows := func(kind string) *pgxmock.Rows {
		return pgxmock.NewRows(columns).
			AddRow(messageID, groupID, "user-2", "bob", "", kind, nil, nil, nil, nil, "hello", time.Now())
	}
	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows("system"))
	mock.ExpectQuery("SELECT message_id, reaction, COUNT").WithArgs([]string{messageID}, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
	requireStatus(t, chatAPI.SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusBadRequest)

	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").WithArgs(messageID).WillReturnRows(messageRows("text"))
	mock.ExpectQuery("SELECT message_id, reaction, COUNT").WithArgs([]string{messageID}, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, chatAPI.SetMessageReaction, reactionRequest(http.MethodPut, "👍"), http.StatusForbidden)

	requireStatus(t, gameAPI.GroupNotifications, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, gameAPI.GroupNotifications, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusForbidden)
}

func TestGroupPhotoAvailabilityFailures(t *testing.T) {
	groupID := "00000000-0000-0000-0000-000000000001"
	nilStoreGame := NewGameAPI(nil, nil, nil, nil, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, nilStoreGame.GroupPhoto, groupPhotoUploadRequest(t, groupID, mustDecodeBase64(onePixelPNG)), http.StatusServiceUnavailable)
	requireStatus(t, nilStoreGame.GroupPhoto, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusServiceUnavailable)

	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").WithArgs(groupID).
		WillReturnError(errors.New("database unavailable"))
	requireStatus(t, gameAPI.GroupPhoto, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1"), http.StatusNotFound)
}
