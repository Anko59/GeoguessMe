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

	mock.ExpectExec("INSERT INTO users").
		WithArgs("user-3", "carol", nil, nil, "hash", "avatar.png", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateUser(context.Background(), &models.User{ID: "user-3", Username: "carol", Password: "hash", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}); err != nil {
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
		WillReturnRows(pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at", "pending_email", "legacy_password_enabled", "oidc_linked"}).
			AddRow("user-1", "alice", nil, "hash", "avatar.png", nil, 0, now, now, "alice@example.test", true, false))
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

func promoteRows(pendingEmail, pendingNormalized string) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).
		AddRow(pendingEmail, pendingNormalized)
}

// TestVerifyEmailTransactionPromotes verifies the verification flow consumes
// the token and promotes the pending claim atomically.
func TestVerifyEmailTransactionPromotes(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("token-hash").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-1", "alice@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-1").
		WillReturnRows(promoteRows("alice@example.test", "alice@example.test"))
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
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-2", "verified@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-2").
		WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow(nil, nil))
	mock.ExpectQuery("SELECT email_normalized FROM users").WithArgs("user-2").
		WillReturnRows(pgxmock.NewRows([]string{"email_normalized"}).AddRow("verified@example.test"))
	mock.ExpectCommit()
	if err := repo.VerifyEmailTransaction(context.Background(), "already-verified-token"); err != nil {
		t.Fatalf("nothing-to-promote VerifyEmailTransaction = %v, want nil", err)
	}

	// A claim conflict surfaces ErrClaimConflict and rolls back so the token
	// is not consumed.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("conflict-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-3", "taken@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-3").
		WillReturnRows(promoteRows("taken@example.test", "taken@example.test"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("taken@example.test", "user-3").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if err := repo.VerifyEmailTransaction(context.Background(), "conflict-token"); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("conflict VerifyEmailTransaction err = %v, want ErrClaimConflict", err)
	}

	// A concurrent promotion that wins after the pre-check is still translated
	// to the same generic claim conflict.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("racing-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-4", "race@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-4").
		WillReturnRows(promoteRows("race@example.test", "race@example.test"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("race@example.test", "user-4").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE users SET email =").WithArgs("race@example.test", "race@example.test", "user-4").
		WillReturnError(&pgconn.PgError{Code: "23505"})
	if err := repo.VerifyEmailTransaction(context.Background(), "racing-token"); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("racing VerifyEmailTransaction err = %v, want ErrClaimConflict", err)
	}

	// A token issued for an older claim cannot promote a replacement address.
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs("stale-claim-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-5", "old@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-5").
		WillReturnRows(promoteRows("new@example.test", "new@example.test"))
	if err := repo.VerifyEmailTransaction(context.Background(), "stale-claim-token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("changed-claim VerifyEmailTransaction err = %v, want ErrTokenInvalid", err)
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

func TestLegacyIdentityMigrationInventoryClassifiesWithoutExposingClaims(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery("SELECT u.email, u.email_verified_at IS NOT NULL").WillReturnRows(
		pgxmock.NewRows([]string{"email", "verified", "pending_email", "linked"}).
			AddRow(" Verified@Example.test ", true, nil, false).
			AddRow(nil, false, "pending@example.test", false).
			AddRow(nil, false, nil, false).
			AddRow("linked@example.test", true, nil, true),
	)
	inventory, err := repo.LegacyIdentityMigrationInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Total != 4 || inventory.Linked != 1 || inventory.Verified != 1 || inventory.Pending != 1 || inventory.Missing != 1 {
		t.Fatalf("unexpected inventory: %+v", inventory)
	}
	if len(inventory.VerifiedEmails) != 1 || inventory.VerifiedEmails[0] != "verified@example.test" {
		t.Fatalf("verified candidates = %v", inventory.VerifiedEmails)
	}
}
