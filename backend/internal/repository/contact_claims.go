package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrClaimConflict is returned when a pending email claim cannot be promoted
// because the same verified address already belongs to another account.
var ErrClaimConflict = errors.New("email claim conflicts with a verified address")

// ErrNothingToPromote is returned when a user has no pending email claim to
// promote.
var ErrNothingToPromote = errors.New("no pending email claim to promote")

// GetUserByVerifiedEmail resolves an account by its VERIFIED email address.
// Pending (unverified) claims never match, so password recovery and
// uniqueness enforcement only ever act on confirmed addresses.
func (r *Repository) GetUserByVerifiedEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email_normalized = $1 AND email IS NOT NULL AND deleted_at IS NULL`
	return scanUser(r.pool.QueryRow(ctx, query, strings.ToLower(strings.TrimSpace(email))))
}

// GetUserByPendingEmail resolves an account that currently holds the address
// as a pending claim. Multiple pending claims for the same address are
// permitted, so this returns at most one match and is informational only.
func (r *Repository) GetUserByPendingEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE pending_email_normalized = $1 AND deleted_at IS NULL ORDER BY created_at LIMIT 1`
	return scanUser(r.pool.QueryRow(ctx, query, strings.ToLower(strings.TrimSpace(email))))
}

// SetPendingEmail records a new pending contact claim without disturbing the
// account's verified address. The verified address stays active until the
// replacement claim is promoted by PromotePendingEmail.
func (r *Repository) SetPendingEmail(ctx context.Context, userID, email string) error {
	normalized := strings.ToLower(strings.TrimSpace(email))
	_, err := r.pool.Exec(ctx, `UPDATE users SET pending_email = $1, pending_email_normalized = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3 AND deleted_at IS NULL`, email, normalized, userID)
	return err
}

// PromotePendingEmail atomically moves the user's pending claim into the
// verified email columns. It fails with ErrClaimConflict when the claimed
// address is already verified by another account, and with
// ErrNothingToPromote when no pending claim exists.
func (r *Repository) PromotePendingEmail(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := promotePendingEmailOn(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// promotePendingEmailOn applies the pending-claim promotion inside an existing
// transaction. It locks the user row, rejects claims that collide with an
// already-verified address, and atomically moves the pending claim into the
// verified email columns. The unique index on email_normalized is the final
// arbiter under concurrency; a unique violation is translated to
// ErrClaimConflict so callers never surface raw constraint errors.
func promotePendingEmailOn(ctx context.Context, tx pgx.Tx, userID string) error {
	var pendingEmail, pendingNormalized sql.NullString
	err := tx.QueryRow(ctx, `SELECT pending_email, pending_email_normalized FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&pendingEmail, &pendingNormalized)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("user not found")
	}
	if err != nil {
		return err
	}
	if !pendingEmail.Valid || !pendingNormalized.Valid {
		return ErrNothingToPromote
	}
	var conflict bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email_normalized = $1 AND id <> $2 AND deleted_at IS NULL)`, pendingNormalized.String, userID).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrClaimConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET email = $1, email_normalized = $2, email_verified_at = CURRENT_TIMESTAMP, pending_email = NULL, pending_email_normalized = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $3`, pendingEmail.String, pendingNormalized.String, userID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrClaimConflict
		}
		return err
	}
	return nil
}

// ResendTargetEmail returns the address a verification email should target:
// the current pending claim, falling back to a verified address. It returns
// nil when the account has no usable contact address. Handlers use this so
// resend only ever targets the current pending claim.
func ResendTargetEmail(user *models.User) *string {
	if user.PendingEmail != "" {
		return &user.PendingEmail
	}
	if user.Email != "" {
		return &user.Email
	}
	return nil
}
