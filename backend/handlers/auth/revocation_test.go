package auth

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
	"golang.org/x/crypto/bcrypt"
)

func TestLogoutFailClosedOnRevocationError(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)

	t.Run("single logout surfaces revocation errors as 500 and clears the cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
		mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnError(errors.New("database unavailable"))
		rr := httptest.NewRecorder()
		api.Logout(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("single logout status = %d, want 500", rr.Code)
		}
		if rr.Header().Get("Set-Cookie") == "" {
			t.Fatal("single logout did not clear the refresh cookie on failure")
		}
	})

	t.Run("logout-all surfaces revocation errors as 500 and clears the cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
		mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE users SET auth_version").WithArgs("user-1").WillReturnError(errors.New("database unavailable"))
		mock.ExpectRollback()
		rr := httptest.NewRecorder()
		api.Logout(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("logout-all status = %d, want 500", rr.Code)
		}
		if rr.Header().Get("Set-Cookie") == "" {
			t.Fatal("logout-all did not clear the refresh cookie on failure")
		}
	})

	t.Run("logout-all without a session is a truthful no-op 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "unknown"})
		mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("unknown")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}))
		rr := httptest.NewRecorder()
		api.Logout(rr, req)
		if rr.Code != http.StatusNoContent || rr.Header().Get("Set-Cookie") == "" {
			t.Fatalf("no-session logout-all = %d, cookie %q", rr.Code, rr.Header().Get("Set-Cookie"))
		}
	})
}

func TestRevocationHandlersKickLiveSockets(t *testing.T) {
	t.Run("logout-all disconnects the user after successful revocation", func(t *testing.T) {
		mock := newAuthMockPool(t)
		kicker := &fakeKicker{}
		api := newAuthAPIWithKicker(t, mock, kicker)
		req := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
		mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE users SET auth_version").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 2))
		mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		rr := httptest.NewRecorder()
		api.Logout(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("logout-all status = %d, want 204", rr.Code)
		}
		if users := kicker.kickedUsers(); len(users) != 1 || users[0] != "user-1" {
			t.Fatalf("kicked users = %v, want [user-1]", users)
		}
	})

	t.Run("logout-all with a nil kicker stays a safe 204", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		req := httptest.NewRequest(http.MethodPost, "/?all=1", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh"})
		mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs(authsvc.HashToken("raw-refresh")).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE users SET auth_version").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 2))
		mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		rr := httptest.NewRecorder()
		api.Logout(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("nil-kicker logout-all status = %d, want 204", rr.Code)
		}
	})

	t.Run("change password disconnects the user", func(t *testing.T) {
		mock := newAuthMockPool(t)
		kicker := &fakeKicker{}
		api := newAuthAPIWithKicker(t, mock, kicker)
		hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE users SET password").WithArgs(pgxmock.AnyArg(), user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		rr := httptest.NewRecorder()
		api.ChangePassword(rr, requestWithUser(http.MethodPost, "/", `{"current_password":"Password123","new_password":"NewPassword123"}`, user.ID))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("change-password status = %d, want 204", rr.Code)
		}
		if users := kicker.kickedUsers(); len(users) != 1 || users[0] != "user-1" {
			t.Fatalf("kicked users = %v, want [user-1]", users)
		}
	})

	t.Run("reset password disconnects the user", func(t *testing.T) {
		mock := newAuthMockPool(t)
		kicker := &fakeKicker{}
		api := newAuthAPIWithKicker(t, mock, kicker)
		tokenHash := authsvc.HashToken("reset-token-raw")
		mock.ExpectBegin()
		mock.ExpectQuery("UPDATE password_reset_tokens").WithArgs(tokenHash).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
		mock.ExpectExec("UPDATE users SET password").WithArgs(pgxmock.AnyArg(), "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()
		rr := httptest.NewRecorder()
		api.ResetPassword(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"reset-token-raw","password":"NewPassword123"}`)))
		if rr.Code != http.StatusOK {
			t.Fatalf("reset-password status = %d, want 200", rr.Code)
		}
		if users := kicker.kickedUsers(); len(users) != 1 || users[0] != "user-1" {
			t.Fatalf("kicked users = %v, want [user-1]", users)
		}
	})

	t.Run("account deletion disconnects the user", func(t *testing.T) {
		mock := newAuthMockPool(t)
		kicker := &fakeKicker{}
		api := newAuthAPIWithKicker(t, mock, kicker)
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
		rr := httptest.NewRecorder()
		api.DeleteAccount(rr, requestWithUser(http.MethodDelete, "/", `{"password":"Password123"}`, user.ID))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", rr.Code)
		}
		if users := kicker.kickedUsers(); len(users) != 1 || users[0] != "user-1" {
			t.Fatalf("kicked users = %v, want [user-1]", users)
		}
	})
}
