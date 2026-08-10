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
	return pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at", "pending_email"}).
		AddRow(user.ID, user.Username, user.Email, user.Password, user.Avatar, user.EmailVerifiedAt, user.AuthVersion, user.CreatedAt, user.UpdatedAt, user.PendingEmail)
}

func TestUserQueriesAndSessionLifecycle(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	user := &models.User{ID: "user-1", Username: "alice", Email: "Alice@Example.test", Password: "hash", Avatar: "avatar.png", AuthVersion: 2, CreatedAt: now, UpdatedAt: now}
	mock.ExpectExec("INSERT INTO users").WithArgs(user.ID, user.Username, user.Email, "alice@example.test", user.Password, user.Avatar, user.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() (*models.User, error){
		"username": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(userRows(user))
			return repo.GetUserByUsername(context.Background(), "alice")
		},
		"email": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(userRows(user))
			return repo.GetUserByEmail(context.Background(), " Alice@Example.test ")
		},
		"id": func() (*models.User, error) {
			mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(userRows(user))
			return repo.GetUserByID(context.Background(), user.ID)
		},
	} {
		got, err := call()
		if err != nil || got == nil || got.ID != user.ID {
			t.Errorf("%s = %+v, %v", name, got, err)
		}
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(2))
	status, err := repo.GetUserAuthStatus(context.Background(), user.ID)
	if err != nil || !status.Active || status.AuthVersion != 2 {
		t.Fatalf("auth status = %+v, %v", status, err)
	}
	mock.ExpectQuery("SELECT auth_version").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	status, err = repo.GetUserAuthStatus(context.Background(), "missing")
	if err != nil || status.Active {
		t.Fatalf("missing auth status = %+v, %v", status, err)
	}

	session := RefreshSession{ID: "session-1", UserID: user.ID, ExpiresAt: now.Add(time.Hour)}
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(session.ID, session.UserID, "token-hash", session.ExpiresAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateRefreshSession(context.Background(), session, "token-hash"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash").WithArgs("token-hash").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.RevokeRefreshSessionByHash(context.Background(), "token-hash"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs("token-hash").WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	owner, err := repo.UserIDByRefreshHash(context.Background(), "token-hash")
	if err != nil || owner != user.ID {
		t.Fatalf("session owner = %q, %v", owner, err)
	}
	mock.ExpectQuery("SELECT user_id FROM refresh_sessions").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	owner, err = repo.UserIDByRefreshHash(context.Background(), "missing")
	if err != nil || owner != "" {
		t.Fatalf("missing session owner = %q, %v", owner, err)
	}
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.BumpAuthVersion(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeAllRefreshSessions(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
}

func TestGetUserScoreStats(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\), COALESCE\\(AVG\\(score\\), 0\\)").
		WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count", "average_score"}).AddRow(int64(7600), int64(3), 2533.33))
	stats, err := repo.GetUserScoreStats(context.Background(), "user-1")
	if err != nil || stats.TotalPoints != 7600 || stats.GuessCount != 3 || stats.AverageScore != 2533.33 {
		t.Fatalf("user score stats = %+v, %v", stats, err)
	}
}

func TestGetGlobalRank(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	// Two players ahead on lifetime points: rank is 3 of 1,943 ranked players.
	mock.ExpectQuery("WITH totals AS").WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(3), int64(1943)))
	stats, err := repo.GetGlobalRank(context.Background(), "user-1")
	if err != nil || stats.Rank != 3 || stats.TotalPlayers != 1943 {
		t.Fatalf("global rank = %+v, %v", stats, err)
	}

	// A player who never guessed is not part of the ranked population.
	mock.ExpectQuery("WITH totals AS").WithArgs("user-2").
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(0), int64(1943)))
	stats, err = repo.GetGlobalRank(context.Background(), "user-2")
	if err != nil || stats.Rank != 0 || stats.TotalPlayers != 1943 {
		t.Fatalf("unranked player = %+v, %v", stats, err)
	}

	// No guesses anywhere: both values are zero.
	mock.ExpectQuery("WITH totals AS").WithArgs("user-3").
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(0), int64(0)))
	stats, err = repo.GetGlobalRank(context.Background(), "user-3")
	if err != nil || stats.Rank != 0 || stats.TotalPlayers != 0 {
		t.Fatalf("empty population = %+v, %v", stats, err)
	}
}

func TestProfileAndPasswordUpdates(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice-new", Email: "alice-new@example.test", Password: "hash", Avatar: "avatar2.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectExec("UPDATE users SET username").WithArgs(user.Username, user.Avatar, user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users SET pending_email").WithArgs(user.Email, user.Email, user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(userRows(user))
	updated, err := repo.UpdateProfile(context.Background(), user.ID, user.Username, user.Email, user.Avatar)
	if err != nil || updated == nil || updated.Avatar != user.Avatar {
		t.Fatalf("UpdateProfile = %+v, %v", updated, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET password").WithArgs("new-hash", user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	if err := repo.ChangePassword(context.Background(), user.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
}

func TestProfileAndPasswordUpdateFailures(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectExec("UPDATE users SET username").WithArgs("alice", "avatar.png", "user-1").WillReturnError(errors.New("profile write failed"))
	if _, err := repo.UpdateProfile(context.Background(), "user-1", "alice", "alice@example.test", "avatar.png"); err == nil {
		t.Fatal("UpdateProfile succeeded after write failure")
	}
	mock.ExpectBegin().WillReturnError(errors.New("transaction unavailable"))
	if err := repo.ChangePassword(context.Background(), "user-1", "hash"); err == nil {
		t.Fatal("ChangePassword succeeded after transaction failure")
	}
	mock.ExpectExec("UPDATE users SET username").WithArgs("alice", "avatar.png", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users SET pending_email").WithArgs("alice@example.test", "alice@example.test", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").WillReturnError(pgx.ErrNoRows)
	if updated, err := repo.UpdateProfile(context.Background(), "user-1", "alice", "alice@example.test", "avatar.png"); err != nil || updated != nil {
		t.Fatalf("UpdateProfile missing user = %+v, %v", updated, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET password").WithArgs("hash", "user-1").WillReturnError(errors.New("password write failed"))
	if err := repo.ChangePassword(context.Background(), "user-1", "hash"); err == nil {
		t.Fatal("ChangePassword succeeded after password write failure")
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET password").WithArgs("hash", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs("user-1").WillReturnError(errors.New("session revoke failed"))
	if err := repo.ChangePassword(context.Background(), "user-1", "hash"); err == nil {
		t.Fatal("ChangePassword succeeded after session revoke failure")
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET password").WithArgs("hash", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnError(errors.New("ticket delete failed"))
	if err := repo.ChangePassword(context.Background(), "user-1", "hash"); err == nil {
		t.Fatal("ChangePassword succeeded after ticket revocation failure")
	}
}

func TestRevokeAllCredentials(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 4))
	mock.ExpectCommit()
	if err := repo.RevokeAllCredentials(context.Background(), "user-1"); err != nil {
		t.Fatalf("revoke all credentials: %v", err)
	}
}

func TestRevokeAllCredentialsRollsBackOnFailure(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id").WithArgs("user-1").WillReturnError(errors.New("session revoke failed"))
	mock.ExpectRollback()
	if err := repo.RevokeAllCredentials(context.Background(), "user-1"); err == nil {
		t.Fatal("RevokeAllCredentials succeeded after session revoke failure")
	}
}

func TestRotateRefreshSessionAndOneTimeTokens(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(now, "presented").WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(userRows(user))
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs("replacement", user.ID, "replacement-hash", now.Add(time.Hour)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	rotated, err := repo.RotateRefreshSession(context.Background(), "presented", "replacement", "replacement-hash", now.Add(time.Hour), now)
	if err != nil || rotated == nil || rotated.ID != user.ID {
		t.Fatalf("rotated user = %+v, %v", rotated, err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE refresh_sessions SET revoked_at").WithArgs(now, "missing").WillReturnError(pgx.ErrNoRows)
	rotated, err = repo.RotateRefreshSession(context.Background(), "missing", "replacement", "hash", now, now)
	if err != nil || rotated != nil {
		t.Fatalf("missing rotation = %+v, %v", rotated, err)
	}

	if err := repo.InsertOneTimeToken(context.Background(), "invalid", "id", "user", "hash", now); err == nil {
		t.Fatal("invalid token table accepted")
	}
	for _, table := range []string{"email_verification_tokens", "password_reset_tokens"} {
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM " + table).WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO "+table).WithArgs("token-id", "user-1", "token-hash", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		if err := repo.InsertOneTimeToken(context.Background(), table, "token-id", "user-1", "token-hash", now); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("bad-token").WillReturnError(pgx.ErrNoRows)
	if err := repo.VerifyEmailTransaction(context.Background(), "bad-token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("invalid verification token = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE password_reset_tokens").WithArgs("bad-token").WillReturnError(pgx.ErrNoRows)
	if _, err := repo.ResetPasswordTransaction(context.Background(), "bad-token", "new-hash"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("invalid reset token = %v", err)
	}
}

func TestUserMutationAndCascade(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM photos").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("photos/a").AddRow("photos/b"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/a").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/b").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	for _, table := range []string{"refresh_sessions", "email_verification_tokens", "password_reset_tokens", "websocket_tickets"} {
		mock.ExpectExec("DELETE FROM " + table).WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}
	mock.ExpectExec("DELETE FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	keys, err := repo.DeleteUserCascade(context.Background(), "user-1")
	if err != nil || len(keys) != 2 {
		t.Fatalf("deleted keys = %v, %v", keys, err)
	}
	mock.ExpectExec("DELETE FROM refresh_sessions WHERE expires_at").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.CleanupAuthTokens(context.Background()); err != nil {
		t.Fatal(err)
	}
}
