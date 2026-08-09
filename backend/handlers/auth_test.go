package handlers

import (
	"bytes"
	"context"
	"errors"
	"geoguessme/internal/auth"
	"geoguessme/internal/chat"
	"geoguessme/internal/email"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/storage"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestSignup(t *testing.T) {
	// Setup
	// Note: In a real app we'd mock the database.
	// For now we assume the DB is running or we skip if not.
	// Actually, unit tests shouldn't depend on external DB.
	// But since I haven't set up dependency injection for the DB,
	// I will write a test that mocks the request structure but might fail if DB isn't there.
	// Ideally we refactor to use interfaces.

	// For this task, I'll focus on the handler signature and bad request validation
	// which doesn't need DB.

	t.Run("Invalid Payload", func(t *testing.T) {
		reqBody := []byte(`{"username": ""}`) // Missing password
		req, _ := http.NewRequestWithContext(context.Background(), "POST", "/signup", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		handler := http.HandlerFunc(Signup)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice"}{"username":"bob"}`))
	var payload SignupRequest
	if decodeJSON(recorder, request, &payload) {
		t.Fatal("decodeJSON accepted trailing JSON")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("decodeJSON status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSessionSetupFailuresReturnServerError(t *testing.T) {
	setupHandlers(t)
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test"}

	// A failed refresh-session insert must not produce an access token or cookie.
	mock := handlerMock(t)
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(assert.AnError)
	recorder := httptest.NewRecorder()
	issueSession(context.Background(), recorder, user)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("issueSession status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	// Token signing failure is surfaced as a server error instead of issuing a
	// cookie with an unusable access token.
	auth.Init("short")
	recorder = httptest.NewRecorder()
	writeSession(recorder, user, "refresh-token")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("writeSession status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestSignupRefreshLogoutAndEmailFlows(t *testing.T) {
	setupHandlers(t)
	Mailer = email.Noop{}
	mock := handlerMock(t)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	noUser := func(pattern string, arg string) {
		mock.ExpectQuery(pattern).WithArgs(arg).WillReturnError(pgx.ErrNoRows)
	}
	noUser("SELECT .*FROM users WHERE username", "alice")
	noUser("SELECT .*FROM users WHERE email_normalized", "alice@example.test")
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), "alice", "alice@example.test", "alice@example.test", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	Signup(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice","email":"alice@example.test","password":"Password123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("signup status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/", nil)
	refreshRequest.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(pgxmock.AnyArg(), auth.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	Refresh(recorder, refreshRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", recorder.Code)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(auth.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	recorder = httptest.NewRecorder()
	Logout(recorder, logoutRequest)
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("logout response = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("request verification status = %d", recorder.Code)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE users SET email_verified_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	VerifyEmail(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"verification-token"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify status = %d", recorder.Code)
	}

	mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs(user.Email).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM password_reset_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO password_reset_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	ForgotPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":"alice@example.test"}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("forgot password status = %d", recorder.Code)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE password_reset_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE users SET password").WithArgs(pgxmock.AnyArg(), user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	ResetPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"reset-token","password":"NewPassword123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset password status = %d", recorder.Code)
	}
}

func TestDeleteAccountSuccess(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM photos").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"storage_key"}))
	for _, table := range []string{"refresh_sessions", "email_verification_tokens", "password_reset_tokens", "websocket_tickets"} {
		mock.ExpectExec("DELETE FROM " + table).WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}
	mock.ExpectExec("DELETE FROM users").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	DeleteAccount(recorder, requestWithUser(http.MethodDelete, "/", `{"password":"Password123"}`, user.ID))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", recorder.Code)
	}
}

func handlerUserRows(user *models.User) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at"}).
		AddRow(user.ID, user.Username, user.Email, user.Password, user.Avatar, user.EmailVerifiedAt, user.AuthVersion, user.CreatedAt, user.UpdatedAt)
}

func TestLoginAndAuthMiddlewareSuccess(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	now := time.Now().UTC()
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png", AuthVersion: 3, CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(handlerUserRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	Login(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice","password":"Password123"}`)))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login response = %d %q", recorder.Code, recorder.Body.String())
	}

	token, err := auth.GenerateAccessToken(user.ID, user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(user.AuthVersion))
	called := false
	AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = GetUserIDFromContext(r) == user.ID
		w.WriteHeader(http.StatusNoContent)
	})(httptest.NewRecorder(), func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}())
	if !called {
		t.Fatal("authenticated handler was not called")
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(user.AuthVersion + 1))
	recorder = httptest.NewRecorder()
	AuthMiddleware(func(http.ResponseWriter, *http.Request) { t.Fatal("revoked session reached handler") })(recorder, func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}())
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", recorder.Code)
	}
	if GetUserIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil)) != "" {
		t.Fatal("anonymous request unexpectedly has a user")
	}
}

func TestProfileReadErrorAndMethodBranches(t *testing.T) {
	setupHandlers(t)
	requireStatus(t, GetProfile, requestWithUser(http.MethodPatch, "/", "", "user-1"), http.StatusMethodNotAllowed)

	mock := handlerMock(t)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, GetProfile, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusUnauthorized)

	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\)").WithArgs(user.ID).WillReturnError(errors.New("stats unavailable"))
	requireStatus(t, GetProfile, requestWithUser(http.MethodGet, "/", "", user.ID), http.StatusInternalServerError)

	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	expectProfileQueries(t, mock, user.ID, 0, 0, 0, 0, 0, 0, 0)
	requireStatus(t, UpdateProfile, requestWithUser(http.MethodGet, "/", "", user.ID), http.StatusOK)
}

func TestGroupAndReadHandlers(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
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

func TestTicketAndUnauthorizedMiddlewareBranches(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO websocket_tickets").WithArgs(pgxmock.AnyArg(), "user-1", groupID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	chatAPI.CreateWebSocketTicket(recorder, requestWithUser(http.MethodPost, "/?group_id="+groupID, "", "user-1"))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("ticket status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	AuthMiddleware(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid token reached handler") })(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d", recorder.Code)
	}

	// A nil hub makes the WebSocket endpoint report chat unavailable. This
	// replaces the old HubInstance nil-reset check: the hub is an injected
	// dependency, not a swappable global.
	requireStatus(t, newChatAPI(t, mock, mustTestStore(t), nil).HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
}

func requestWithUser(method, target, body, userID string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	return request.WithContext(context.WithValue(request.Context(), userIDKey, userID))
}

func mustTestStore(t *testing.T) storage.ObjectStore {
	t.Helper()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// newChatAPI builds the migrated chat transport on the caller's mock pool
// (the same pool serves auth membership checks and chat persistence), with the
// given store and hub. It replaces the package globals the old chat handlers
// read: MediaStore, RuntimeConfig, and HubInstance.
func newChatAPI(t *testing.T, mock pgxmock.PgxPoolIface, store storage.ObjectStore, hub *chat.Hub) *ChatAPI {
	t.Helper()
	return NewChatAPI(chatrepo.NewRepository(mock), store, handlerConfig(), hub, time.Now, repository.NewRepository(mock))
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

func requireStatus(t *testing.T, handler http.HandlerFunc, request *http.Request, status int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != status {
		t.Fatalf("%s %s status = %d, want %d (%s)", request.Method, request.URL.Path, recorder.Code, status, recorder.Body.String())
	}
}

func TestHandlersRejectUnsupportedMethods(t *testing.T) {
	setupHandlers(t)
	groupsAPI := NewGroupAPI(stubGroupReader{})
	mock := handlerMock(t)
	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	gameAPI := newGameAPI(t, mock)
	tests := []struct {
		name string
		hand http.HandlerFunc
	}{
		{"signup", Signup}, {"login", Login}, {"refresh", Refresh}, {"logout", Logout},
		{"request verification", RequestVerification}, {"verify email", VerifyEmail}, {"forgot password", ForgotPassword},
		{"reset password", ResetPassword}, {"change password", ChangePassword}, {"delete account", DeleteAccount}, {"create group", gameAPI.CreateGroup},
		{"join group", gameAPI.JoinGroup}, {"leaderboard", gameAPI.GetLeaderboard}, {"ticket", chatAPI.CreateWebSocketTicket},
		{"guess", gameAPI.SubmitChallengeGuess}, {"results", gameAPI.GetChallengeResults}, {"messages", chatAPI.GetGroupMessages},
		{"group details", gameAPI.GetGroupDetails}, {"group members", gameAPI.GetGroupMembers}, {"user groups", groupsAPI.GetUserGroups},
		{"upload", gameAPI.UploadPhoto}, {"accept", gameAPI.AcceptChallenge}, {"media", gameAPI.ServeChallengeMedia},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			requireStatus(t, testCase.hand, requestWithUser(http.MethodPatch, "/", `{}`, "user-1"), http.StatusMethodNotAllowed)
		})
	}
}

func TestAuthValidationAndUnauthenticatedBranches(t *testing.T) {
	setupHandlers(t)
	requireStatus(t, Signup, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":""}`)), http.StatusBadRequest)
	requireStatus(t, Login, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":`)), http.StatusBadRequest)
	requireStatus(t, Refresh, httptest.NewRequest(http.MethodPost, "/", nil), http.StatusUnauthorized)
	requireStatus(t, VerifyEmail, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`)), http.StatusBadRequest)
	requireStatus(t, ForgotPassword, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":`)), http.StatusBadRequest)
	requireStatus(t, ResetPassword, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"x","password":"short"}`)), http.StatusBadRequest)
	mock := handlerMock(t)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, DeleteAccount, requestWithUser(http.MethodDelete, "/", `{}`, "user-1"), http.StatusUnauthorized)

	nilHubAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	requireStatus(t, nilHubAPI.HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
	hubAPI := newChatAPI(t, mock, mustTestStore(t), chat.NewHub(nil, nil))
	requireStatus(t, hubAPI.HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusUnauthorized)

	gameAPI := newGameAPI(t, mock)
	requireStatus(t, nilHubAPI.CreateWebSocketTicket, requestWithUser(http.MethodPost, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetLeaderboard, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetLeaderboard, requestWithUser(http.MethodGet, "/?group_id=group-1&period=year", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, nilHubAPI.GetGroupMessages, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetGroupDetails, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetGroupMembers, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.SubmitChallengeGuess, requestWithUser(http.MethodPost, "/", `{}`, "user-1"), http.StatusBadRequest)
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs("").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, gameAPI.GetChallengeResults, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusNotFound)
	requireStatus(t, gameAPI.AcceptChallenge, requestWithUser(http.MethodPost, "/", `{}`, "user-1"), http.StatusBadRequest)
	repos := repository.NewRepository(mock)
	nilStoreGame := NewGameAPI(repos.Groups, repos.Chat, repos, nil, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, nilStoreGame.ServeChallengeMedia, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusServiceUnavailable)
}

func TestGroupAndUploadValidation(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	gameAPI := newGameAPI(t, mock)
	requireStatus(t, gameAPI.CreateGroup, requestWithUser(http.MethodPost, "/", `{"name":""}`, "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.JoinGroup, requestWithUser(http.MethodPost, "/", `{"code":"bad"}`, "user-1"), http.StatusBadRequest)
	repos := repository.NewRepository(mock)
	nilStoreGame := NewGameAPI(repos.Groups, repos.Chat, repos, nil, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, nilStoreGame.UploadPhoto, requestWithUser(http.MethodPost, "/", "", "user-1"), http.StatusServiceUnavailable)

	validationGame := NewGameAPI(repos.Groups, repos.Chat, repos, &validationStore{}, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, validationGame.UploadPhoto, requestWithUser(http.MethodPost, "/", "not-multipart", "user-1"), http.StatusBadRequest)
	if err := validateID("", "id"); err == nil {
		t.Fatal("empty identifier accepted")
	}
	if err := validateID("not-a-uuid", "id"); err == nil {
		t.Fatal("invalid identifier accepted")
	}
	if err := validateID("00000000-0000-0000-0000-000000000001", "id"); err != nil {
		t.Fatal(err)
	}
	if mediaURL(&models.Photo{ID: "photo-1"}, false) != "/api/v1/challenges/photo-1/media" || mediaURL(&models.Photo{ID: "photo-1"}, true) == mediaURL(&models.Photo{ID: "photo-1"}, false) {
		t.Fatal("media URLs are not distinct")
	}
}
