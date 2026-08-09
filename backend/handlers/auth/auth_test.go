package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
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
	return NewAuthAPI(repository.NewRepository(mock), cfg, store, email.Noop{}, authsvc.NewService(cfg.JWTSecret, "geoguessme", "geoguessme-web", cfg.AccessTokenTTL))
}

func requestWithUser(method, target, body, userID string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	return request.WithContext(handlers.WithUserID(request.Context(), userID))
}

func handlerUserRows(user *models.User) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at"}).
		AddRow(user.ID, user.Username, user.Email, user.Password, user.Avatar, user.EmailVerifiedAt, user.AuthVersion, user.CreatedAt, user.UpdatedAt)
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

	// Signup: uniqueness checks, account insert, verification token, session.
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), "alice", "alice@example.test", "alice@example.test", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
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

	// Logout-all revokes every session and bumps the auth version.
	logoutRequest := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	recorder = httptest.NewRecorder()
	api.Logout(recorder, logoutRequest)
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("logout response = %d", recorder.Code)
	}

	// RequestVerification re-sends the link for an unverified account.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("request verification status = %d", recorder.Code)
	}

	// VerifyEmail consumes the token and marks the account verified.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE users SET email_verified_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
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
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.ResetPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"reset-token","password":"NewPassword123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset password status = %d", recorder.Code)
	}
}

func userColumnsForQuery() []string {
	return []string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at"}
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
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(handlerUserRows(user))
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
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(user.AuthVersion))
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
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(user.AuthVersion + 1))
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
