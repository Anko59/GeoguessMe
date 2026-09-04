package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/email"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

// authConfig is the test configuration shared by the auth handler tests.
func authConfig() *config.Config {
	return &config.Config{
		Environment: "test", PublicURL: "http://localhost:8080", JWTSecret: "test_secret_key_at_least_32_characters_long",
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, VerificationTTL: 24 * time.Hour, ResetTTL: time.Hour,
		PasswordHashCost: 4, UploadMaxBytes: 5 * 1024 * 1024, AvatarMaxBytes: 25 * 1024 * 1024, UploadMaxPixels: 100000, ChallengeTTL: time.Hour,
		ViewWindow: time.Minute, LocationHide: 48 * time.Hour, PhotoRetention: 24 * time.Hour, AllowedOrigins: []string{"http://localhost:8080"},
	}
}

func newAuthMockPool(t *testing.T) pgxmock.PgxPoolIface {
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

// newAuthAPI builds the migrated auth transport on the caller's mock pool with
// an isolated repository, config, store, and token service. It replaces the
// package globals the old auth handlers read: RuntimeConfig, MediaStore,
// Mailer, and the package-level token state (auth.Init).
func newAuthAPI(t *testing.T, mock pgxmock.PgxPoolIface, store storage.ObjectStore) *AuthAPI {
	t.Helper()
	cfg := authConfig()
	return NewAuthAPI(repository.NewRepository(mock), cfg, store, email.Noop{}, authsvc.NewService(cfg.JWTSecret, "geoguessme", "geoguessme-web", cfg.AccessTokenTTL), nil)
}

// newAuthAPIWithKicker builds the auth transport with an explicit socket
// kicker so tests can assert credential revocation closes live sockets.
func newAuthAPIWithKicker(t *testing.T, mock pgxmock.PgxPoolIface, kicker chatHub.SocketKicker) *AuthAPI {
	t.Helper()
	cfg := authConfig()
	return NewAuthAPI(repository.NewRepository(mock), cfg, nil, email.Noop{}, authsvc.NewService(cfg.JWTSecret, "geoguessme", "geoguessme-web", cfg.AccessTokenTTL), kicker)
}

// fakeKicker records DisconnectUser calls so tests can assert that credential
// revocation closes live sockets. It is safe for concurrent use.
type fakeKicker struct {
	mu    sync.Mutex
	users []string
}

func (f *fakeKicker) DisconnectUser(userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userID)
}

func (f *fakeKicker) DisconnectUserInGroup(userID, groupID string) {}

func (f *fakeKicker) kickedUsers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.users...)
}

func requestWithUser(method, target, body, userID string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	return request.WithContext(handlers.WithUserID(request.Context(), userID))
}

func handlerUserRows(user *models.User) *pgxmock.Rows {
	passwordEnabled := user.PasswordEnabled || user.Password != "!"
	return pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at", "pending_email", "legacy_password_enabled", "oidc_linked"}).
		AddRow(user.ID, user.Username, user.Email, user.Password, user.Avatar, user.EmailVerifiedAt, user.AuthVersion, user.CreatedAt, user.UpdatedAt, user.PendingEmail, passwordEnabled, user.OIDCLinked)
}

func TestSignup(t *testing.T) {
	t.Run("Invalid Payload", func(t *testing.T) {
		api := newAuthAPI(t, newAuthMockPool(t), nil)
		reqBody := []byte(`{"username": ""}`) // Missing password
		req, _ := http.NewRequestWithContext(context.Background(), "POST", "/signup", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		api.Signup(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice"}{"username":"bob"}`))
	var payload SignupRequest
	if handlers.DecodeJSON(recorder, request, &payload) {
		t.Fatal("DecodeJSON accepted trailing JSON")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("DecodeJSON status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSessionSetupFailuresReturnServerError(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png"}
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(assert.AnError)
	recorder := httptest.NewRecorder()
	api.issueSession(httptest.NewRequest(http.MethodPost, "/", nil).Context(), recorder, user)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("session failure status = %d", recorder.Code)
	}
}

func TestSignupRefreshLogoutAndEmailFlows(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}

	// Signup: username uniqueness, account insert, verification token, session.
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), "alice", "alice@example.test", "alice@example.test", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "alice@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	api.Signup(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice","email":"alice@example.test","password":"StrongPassword123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("signup status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// Refresh rotates the presented session.
	refreshRequest := httptest.NewRequest(http.MethodPost, "/", nil)
	refreshRequest.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(pgxmock.AnyArg(), authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.Refresh(recorder, refreshRequest)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("refresh status = %d", recorder.Code)
	}

	// Logout-all atomically revokes every session, bumps the auth version, and
	// deletes outstanding WebSocket tickets.
	logoutRequest := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.Logout(recorder, logoutRequest)
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("logout response = %d", recorder.Code)
	}

	// RequestVerification re-sends the link for an unverified account.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), "alice@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("request verification status = %d", recorder.Code)
	}

	// VerifyEmail consumes the token and promotes the pending email claim.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow(user.ID, "alice@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow("alice@example.test", "alice@example.test"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("alice@example.test", user.ID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE users SET email =").WithArgs("alice@example.test", "alice@example.test", user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.VerifyEmail(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"verification-token"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify status = %d", recorder.Code)
	}

	// ForgotPassword sends a reset token for a registered account.
	mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs(user.Email).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM password_reset_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO password_reset_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.ForgotPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":"alice@example.test"}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("forgot password status = %d", recorder.Code)
	}

	// ResetPassword consumes the token and installs a new password.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE password_reset_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE users SET password").WithArgs(pgxmock.AnyArg(), user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.ResetPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"reset-token","password":"NewPassword123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset password status = %d", recorder.Code)
	}
}

func userColumnsForQuery() []string {
	return []string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at", "pending_email", "legacy_password_enabled", "oidc_linked"}
}

func TestDeleteAccountSuccess(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
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
	api.DeleteAccount(recorder, requestWithUser(http.MethodDelete, "/", `{"password":"Password123"}`, user.ID))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", recorder.Code)
	}
}

func TestLoginAndAuthMiddlewareSuccess(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	now := time.Now().UTC()
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png", AuthVersion: 3, CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users").WithArgs("alice", "alice").WillReturnRows(handlerUserRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	api.Login(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice","password":"Password123"}`)))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login response = %d %q", recorder.Code, recorder.Body.String())
	}

	token, err := api.svc.GenerateAccessToken(user.ID, user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version", "oidc_linked"}).AddRow(user.AuthVersion, false))
	called := false
	api.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = handlers.GetUserIDFromContext(r) == user.ID
		w.WriteHeader(http.StatusNoContent)
	})(httptest.NewRecorder(), func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}())
	if !called {
		t.Fatal("authenticated handler was not called")
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version", "oidc_linked"}).AddRow(user.AuthVersion+1, false))
	recorder = httptest.NewRecorder()
	api.AuthMiddleware(func(http.ResponseWriter, *http.Request) { t.Fatal("revoked session reached handler") })(recorder, func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}())
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", recorder.Code)
	}
	if handlers.GetUserIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil)) != "" {
		t.Fatal("anonymous request unexpectedly has a user")
	}
}

func TestOIDCEnabledRetiresSignupAndLimitsPasswordLoginToMigration(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	api.cfg.OIDCEnabled = true

	recorder := httptest.NewRecorder()
	api.Signup(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBufferString(`{"username":"new-player","password":"Password123"}`)))
	if recorder.Code != http.StatusGone || !strings.Contains(recorder.Body.String(), `"code":"legacy_signup_disabled"`) {
		t.Fatalf("legacy signup = %d %q", recorder.Code, recorder.Body.String())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	legacy := &models.User{ID: "legacy-user", Username: "legacy", Password: string(hash), PasswordEnabled: true, Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users").WithArgs("legacy", "legacy").WillReturnRows(handlerUserRows(legacy))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), legacy.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder = httptest.NewRecorder()
	api.Login(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"legacy","password":"Password123"}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"migration_required":true`) {
		t.Fatalf("migration login = %d %q", recorder.Code, recorder.Body.String())
	}

	linked := &models.User{ID: "linked-user", Username: "linked", Password: string(hash), PasswordEnabled: true, OIDCLinked: true, Avatar: "avatar.png"}
	mock.ExpectQuery("SELECT .*FROM users").WithArgs("linked", "linked").WillReturnRows(handlerUserRows(linked))
	recorder = httptest.NewRecorder()
	api.Login(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"linked","password":"Password123"}`)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("linked password login = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestOIDCDisabledRestoresLinkedLegacyPasswordLogin(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	api.cfg.OIDCEnabled = false
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	linked := &models.User{
		ID: "linked-user", Username: "linked", Password: string(hash), PasswordEnabled: true,
		OIDCLinked: true, Avatar: "avatar.png",
	}
	mock.ExpectQuery("SELECT .*FROM users").WithArgs("linked", "linked").WillReturnRows(handlerUserRows(linked))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), linked.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	api.Login(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"linked","password":"Password123"}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"password_login_enabled":true`) || strings.Contains(recorder.Body.String(), `"migration_required":true`) {
		t.Fatalf("rollback login = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestLegacyReadOnlyMiddleware(t *testing.T) {
	api := newAuthAPI(t, newAuthMockPool(t), nil)
	called := false
	next := api.LegacyReadOnlyMiddleware(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	request := func(method, path string, migrationRequired bool) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		return r.WithContext(handlers.WithMigrationRequired(r.Context(), migrationRequired))
	}

	for _, allowed := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/user/groups"},
		{http.MethodHead, "/api/v1/group/photo"},
		{http.MethodPost, "/api/v1/auth/oidc/link"},
		{http.MethodPost, "/api/v1/auth/verify/request"},
		{http.MethodDelete, "/api/v1/auth/account"},
	} {
		called = false
		recorder := httptest.NewRecorder()
		next(recorder, request(allowed.method, allowed.path, true))
		if !called || recorder.Code != http.StatusNoContent {
			t.Fatalf("%s %s was not allowed: status=%d called=%v", allowed.method, allowed.path, recorder.Code, called)
		}
	}

	called = false
	recorder := httptest.NewRecorder()
	next(recorder, request(http.MethodPost, "/api/v1/challenges/photo-1/guess", true))
	if called || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"migration_required"`) {
		t.Fatalf("legacy write = %d %q called=%v", recorder.Code, recorder.Body.String(), called)
	}

	called = false
	recorder = httptest.NewRecorder()
	next(recorder, request(http.MethodPost, "/api/v1/challenges/photo-1/guess", false))
	if !called || recorder.Code != http.StatusNoContent {
		t.Fatalf("linked write = %d called=%v", recorder.Code, called)
	}
}

func TestProfileReadErrorAndMethodBranches(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	requireStatus(t, api.GetProfile, requestWithUser(http.MethodPatch, "/", "", "user-1"), http.StatusMethodNotAllowed)

	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, api.GetProfile, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusUnauthorized)

	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\)").WithArgs(user.ID).WillReturnError(errors.New("stats unavailable"))
	requireStatus(t, api.GetProfile, requestWithUser(http.MethodGet, "/", "", user.ID), http.StatusInternalServerError)

	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	expectProfileQueries(t, mock, user.ID, 0, 0, 0, 0, 0, 0, 0)
	requireStatus(t, api.UpdateProfile, requestWithUser(http.MethodGet, "/", "", user.ID), http.StatusOK)
}

func requireStatus(t *testing.T, handler http.HandlerFunc, request *http.Request, status int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != status {
		t.Fatalf("%s %s status = %d, want %d (%s)", request.Method, request.URL.Path, recorder.Code, status, recorder.Body.String())
	}
}

func TestAuthHandlersRejectUnsupportedMethods(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	tests := []struct {
		name string
		hand http.HandlerFunc
	}{
		{"signup", api.Signup}, {"login", api.Login}, {"refresh", api.Refresh}, {"logout", api.Logout},
		{"request verification", api.RequestVerification}, {"verify email", api.VerifyEmail}, {"forgot password", api.ForgotPassword},
		{"reset password", api.ResetPassword}, {"change password", api.ChangePassword}, {"delete account", api.DeleteAccount},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			requireStatus(t, testCase.hand, requestWithUser(http.MethodPatch, "/", `{}`, "user-1"), http.StatusMethodNotAllowed)
		})
	}
}

func TestAuthValidationAndUnauthenticatedBranches(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	requireStatus(t, api.Signup, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":""}`)), http.StatusBadRequest)
	requireStatus(t, api.Login, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":`)), http.StatusBadRequest)
	requireStatus(t, api.Refresh, httptest.NewRequest(http.MethodPost, "/", nil), http.StatusUnauthorized)
	requireStatus(t, api.VerifyEmail, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`)), http.StatusBadRequest)
	requireStatus(t, api.ForgotPassword, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":`)), http.StatusBadRequest)
	requireStatus(t, api.ResetPassword, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"x","password":"short"}`)), http.StatusBadRequest)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, api.DeleteAccount, requestWithUser(http.MethodDelete, "/", `{}`, "user-1"), http.StatusUnauthorized)
}
