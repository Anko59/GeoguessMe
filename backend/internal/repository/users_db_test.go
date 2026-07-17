package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func userRows(user *models.User) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at"}).
		AddRow(user.ID, user.Username, user.Email, user.Password, user.Avatar, user.EmailVerifiedAt, user.AuthVersion, user.CreatedAt, user.UpdatedAt)
}

func TestUserQueriesAndSessionLifecycle(t *testing.T) {
	mock := newMockPool(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	user := &models.User{ID: "user-1", Username: "alice", Email: "Alice@Example.test", Password: "hash", Avatar: "avatar.png", AuthVersion: 2, CreatedAt: now, UpdatedAt: now}
	mock.ExpectExec("INSERT INTO users").WithArgs(user.ID, user.Username, user.Email, "alice@example.test", user.Password, user.Avatar, user.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := CreateUserContext(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() (*models.User, error){
		"username": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(userRows(user))
			return GetUserByUsernameContext(context.Background(), "alice")
		},
		"email": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(userRows(user))
			return GetUserByEmailContext(context.Background(), " Alice@Example.test ")
		},
		"id": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(userRows(user))
			return GetUserByID(context.Background(), user.ID)
		},
	} {
		got, err := call()
		if err != nil || got == nil || got.ID != user.ID {
			t.Errorf("%s = %+v, %v", name, got, err)
		}
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(2))
	status, err := GetUserAuthStatus(context.Background(), user.ID)
	if err != nil || !status.Active || status.AuthVersion != 2 {
		t.Fatalf("auth status = %+v, %v", status, err)
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	status, err = GetUserAuthStatus(context.Background(), "missing")
	if err != nil || status.Active {
		t.Fatalf("missing auth status = %+v, %v", status, err)
	}

	session := RefreshSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(session.ID, session.UserID, "token-hash", session.ExpiresAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := CreateRefreshSession(context.Background(), session, "token-hash"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id").WithArgs(session.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash").WithArgs("token-hash").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := RevokeRefreshSession(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if err := RevokeRefreshSessionByHash(context.Background(), "token-hash"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs("token-hash").WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	owner, err := UserIDByRefreshHash(context.Background(), "token-hash")
	if err != nil || owner != user.ID {
		t.Fatalf("session owner = %q, %v", owner, err)
	}
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	owner, err = UserIDByRefreshHash(context.Background(), "missing")
	if err != nil || owner != "" {
		t.Fatalf("missing session owner = %q, %v", owner, err)
	}
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := BumpAuthVersion(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	if err := RevokeAllRefreshSessions(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRotateRefreshSessionAndOneTimeTokens(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(now, "presented").WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(userRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs("replacement", user.ID, "replacement-hash", now.Add(time.Hour)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	rotated, err := RotateRefreshSession(context.Background(), "presented", "replacement", "replacement-hash", now.Add(time.Hour), now)
	if err != nil || rotated == nil || rotated.ID != user.ID {
		t.Fatalf("rotated user = %+v, %v", rotated, err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(now, "missing").WillReturnError(pgx.ErrNoRows)
	rotated, err = RotateRefreshSession(context.Background(), "missing", "replacement", "hash", now, now)
	if err != nil || rotated != nil {
		t.Fatalf("missing rotation = %+v, %v", rotated, err)
	}

	if err := InsertOneTimeToken(context.Background(), "invalid", "id", "user", "hash", now); err == nil {
		t.Fatal("invalid token table accepted")
	}
	for _, table := range []string{"email_verification_tokens", "password_reset_tokens"} {
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM " + table).WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO "+table).WithArgs("token-id", "user-1", "token-hash", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		if err := InsertOneTimeToken(context.Background(), table, "token-id", "user-1", "token-hash", now); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("bad-token").WillReturnError(pgx.ErrNoRows)
	if err := VerifyEmailTransaction(context.Background(), "bad-token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("invalid verification token = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE password_reset_tokens").WithArgs("bad-token").WillReturnError(pgx.ErrNoRows)
	if err := ResetPasswordTransaction(context.Background(), "bad-token", "new-hash"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("invalid reset token = %v", err)
	}
}

func TestUserMutationAndCascade(t *testing.T) {
	mock := newMockPool(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM photos").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("photos/a").AddRow("photos/b"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/a").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/b").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	for _, table := range []string{"refresh_sessions", "email_verification_tokens", "password_reset_tokens", "websocket_tickets"} {
		mock.ExpectExec("DELETE FROM " + table).WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}
	mock.ExpectExec("DELETE FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	keys, err := DeleteUserCascade(context.Background(), "user-1")
	if err != nil || len(keys) != 2 {
		t.Fatalf("deleted keys = %v, %v", keys, err)
	}
	mock.ExpectExec("DELETE FROM refresh_sessions WHERE expires_at").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := CleanupAuthTokens(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE users SET email_verified_at").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users SET password").WithArgs("new-hash", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := MarkEmailVerified(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePassword(context.Background(), "user-1", "new-hash"); err != nil {
		t.Fatal(err)
	}
}
