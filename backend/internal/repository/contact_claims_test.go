package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

// TestCreateUserStoresPendingClaim verifies that a new account's submitted
// address lands in the pending columns while the verified email columns stay
// NULL (unverified accounts have no authorization identity).
func TestCreateUserStoresPendingClaim(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	user := &models.User{ID: "user-1", Username: "alice", PendingEmail: "Alice@Example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectExec("INSERT INTO users").
		WithArgs(user.ID, user.Username, user.PendingEmail, "alice@example.test", user.Password, user.Avatar, user.CreatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}

	// Legacy-style callers that only set Email are still persisted as a claim.
	mock.ExpectExec("INSERT INTO users").
		WithArgs("user-2", "bob", "bob@example.test", "bob@example.test", "hash", "avatar.png", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateUser(context.Background(), &models.User{ID: "user-2", Username: "bob", Email: "bob@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

// TestScanUserNullEmail verifies a NULL verified email scans to an empty
// string (unverified account) and a NULL pending claim stays empty.
func TestScanUserNullEmail(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at", "pending_email"}).
			AddRow("user-1", "alice", nil, "hash", "avatar.png", nil, 0, now, now, "alice@example.test"))
	user, err := repo.GetUserByID(context.Background(), "user-1")
	if err != nil || user == nil {
		t.Fatalf("GetUserByID = %+v, %v", user, err)
	}
	if user.Email != "" {
		t.Fatalf("unverified account email = %q, want empty", user.Email)
	}
	if user.EmailVerifiedAt != nil {
		t.Fatalf("unverified account verified_at = %v, want nil", user.EmailVerifiedAt)
	}
	if user.PendingEmail != "alice@example.test" {
		t.Fatalf("pending email = %q", user.PendingEmail)
	}
}

// TestGetUserByVerifiedEmailOnlyMatchesVerified verifies recovery lookups can
// only ever resolve confirmed addresses.
func TestGetUserByVerifiedEmailOnlyMatchesVerified(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").
		WillReturnRows(userRows(user))
	got, err := repo.GetUserByVerifiedEmail(context.Background(), " Alice@Example.test ")
	if err != nil || got == nil || got.ID != user.ID {
		t.Fatalf("GetUserByVerifiedEmail = %+v, %v", got, err)
	}
	// No verified match (a pending claim must not be found here).
	mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("nobody@example.test").
		WillReturnError(pgx.ErrNoRows)
	got, err = repo.GetUserByVerifiedEmail(context.Background(), "nobody@example.test")
	if err != nil || got != nil {
		t.Fatalf("unmatched verified lookup = %+v, %v", got, err)
	}
}

// TestGetUserByPendingEmail verifies the informational pending-claim lookup.
func TestGetUserByPendingEmail(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", PendingEmail: "alice@example.test", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE pending_email_normalized").WithArgs("alice@example.test").
		WillReturnRows(userRows(user))
	got, err := repo.GetUserByPendingEmail(context.Background(), "Alice@Example.test")
	if err != nil || got == nil || got.ID != user.ID {
		t.Fatalf("GetUserByPendingEmail = %+v, %v", got, err)
	}
}

func promoteRows(pendingEmail, pendingNormalized string, verifiedAt *time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).
		AddRow(pendingEmail, pendingNormalized)
}

// TestPromotePendingEmail verifies the atomic claim promotion and the conflict
// and nothing-to-promote error paths.
func TestPromotePendingEmail(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)

	t.Run("promotes when clear", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-1").
			WillReturnRows(promoteRows("alice@example.test", "alice@example.test", nil))
		mock.ExpectQuery("SELECT EXISTS").WithArgs("alice@example.test", "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("UPDATE users SET email =").WithArgs("alice@example.test", "alice@example.test", "user-1").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()
		if err := repo.PromotePendingEmail(context.Background(), "user-1"); err != nil {
			t.Fatalf("promote = %v", err)
		}
	})

	t.Run("conflicts with verified address elsewhere", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-2").
			WillReturnRows(promoteRows("taken@example.test", "taken@example.test", nil))
		mock.ExpectQuery("SELECT EXISTS").WithArgs("taken@example.test", "user-2").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		if err := repo.PromotePendingEmail(context.Background(), "user-2"); !errors.Is(err, ErrClaimConflict) {
			t.Fatalf("conflict promote err = %v, want ErrClaimConflict", err)
		}
	})

	t.Run("nothing to promote", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-3").
			WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow(nil, nil))
		if err := repo.PromotePendingEmail(context.Background(), "user-3"); !errors.Is(err, ErrNothingToPromote) {
			t.Fatalf("noop promote err = %v, want ErrNothingToPromote", err)
		}
	})

	t.Run("unique violation translates to conflict", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-4").
			WillReturnRows(promoteRows("race@example.test", "race@example.test", nil))
		mock.ExpectQuery("SELECT EXISTS").WithArgs("race@example.test", "user-4").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("UPDATE users SET email =").WithArgs("race@example.test", "race@example.test", "user-4").
			WillReturnError(&pgconn.PgError{Code: "23505"})
		if err := repo.PromotePendingEmail(context.Background(), "user-4"); !errors.Is(err, ErrClaimConflict) {
			t.Fatalf("unique violation promote err = %v, want ErrClaimConflict", err)
		}
	})
}

// TestSetPendingEmailKeepsVerifiedAddress verifies a replacement claim never
// disturbs the current verified address.
func TestSetPendingEmailKeepsVerifiedAddress(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectExec("UPDATE users SET pending_email").WithArgs(" New@Example.test ", "new@example.test", "user-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.SetPendingEmail(context.Background(), "user-1", " New@Example.test "); err != nil {
		t.Fatalf("SetPendingEmail = %v", err)
	}
}

// TestVerifyEmailTransactionPromotes verifies the verification flow consumes
// the token and promotes the pending claim atomically.
func TestVerifyEmailTransactionPromotes(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("token-hash").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-1").
		WillReturnRows(promoteRows("alice@example.test", "alice@example.test", nil))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("alice@example.test", "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE users SET email =").WithArgs("alice@example.test", "alice@example.test", "user-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	if err := repo.VerifyEmailTransaction(context.Background(), "token-hash"); err != nil {
		t.Fatalf("VerifyEmailTransaction = %v", err)
	}

	// Invalid token still surfaces ErrTokenInvalid.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("bad-token").WillReturnError(pgx.ErrNoRows)
	if err := repo.VerifyEmailTransaction(context.Background(), "bad-token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("invalid token err = %v, want ErrTokenInvalid", err)
	}

	// A verified account with no pending claim is an idempotent success: the
	// token is consumed and the transaction commits so re-verification is a
	// harmless no-op rather than an error.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("already-verified-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-2"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-2").
		WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow(nil, nil))
	mock.ExpectCommit()
	if err := repo.VerifyEmailTransaction(context.Background(), "already-verified-token"); err != nil {
		t.Fatalf("nothing-to-promote VerifyEmailTransaction = %v, want nil", err)
	}

	// A claim conflict surfaces ErrClaimConflict and rolls back so the token
	// is not consumed.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("conflict-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-3"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-3").
		WillReturnRows(promoteRows("taken@example.test", "taken@example.test", nil))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("taken@example.test", "user-3").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if err := repo.VerifyEmailTransaction(context.Background(), "conflict-token"); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("conflict VerifyEmailTransaction err = %v, want ErrClaimConflict", err)
	}
}

// TestResendTargetEmail verifies the resend targeting helper prefers the
// pending claim over the verified address.
func TestResendTargetEmail(t *testing.T) {
	if got := ResendTargetEmail(&models.User{PendingEmail: "p@example.test", Email: "v@example.test"}); got == nil || *got != "p@example.test" {
		t.Fatalf("pending target = %v", got)
	}
	if got := ResendTargetEmail(&models.User{Email: "v@example.test"}); got == nil || *got != "v@example.test" {
		t.Fatalf("verified target = %v", got)
	}
	if got := ResendTargetEmail(&models.User{}); got != nil {
		t.Fatalf("no-address target = %v, want nil", got)
	}
}
