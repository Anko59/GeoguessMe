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

// ErrUserNotFound is returned when a claim mutation targets no active user.
var ErrUserNotFound = errors.New("user not found")

// ErrAmbiguousEmailClaim prevents recovery from selecting one of multiple
// unverified accounts that happen to claim the same address.
var ErrAmbiguousEmailClaim = errors.New("email claim matches multiple accounts")

// GetUserByVerifiedEmail resolves an account by its VERIFIED email address.
// Pending (unverified) claims never match, so password recovery and
// uniqueness enforcement only ever act on confirmed addresses.
func (r *Repository) GetUserByVerifiedEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email_normalized = $1 AND email IS NOT NULL AND deleted_at IS NULL`
	return scanUser(r.pool.QueryRow(ctx, query, strings.ToLower(strings.TrimSpace(email))))
}

// GetUsersByLoginIdentifier resolves the legacy accounts that could match a
// migration login. Usernames are exact matches; verified and pending email
// claims are normalized matches. Multiple pending claims are possible because
// they are not account identities until verified, so callers must authenticate
// every candidate and reject an ambiguous password match.
func (r *Repository) GetUsersByLoginIdentifier(ctx context.Context, identifier string) ([]*models.User, error) {
	trimmed := strings.TrimSpace(identifier)
	normalized := strings.ToLower(trimmed)
	query := `SELECT ` + userColumns + `
		FROM users
		WHERE deleted_at IS NULL
		  AND (username = $1 OR email_normalized = $2 OR pending_email_normalized = $2)
		ORDER BY CASE
			WHEN username = $1 THEN 0
			WHEN email_normalized = $2 THEN 1
			ELSE 2
		END, created_at, id`
	rows, err := r.pool.Query(ctx, query, trimmed, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]*models.User, 0, 1)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// GetUserByPendingEmail resolves the one pending claim that may be used to
// start email ownership recovery. Ambiguous pending claims are rejected rather
// than selecting an account arbitrarily; the caller receives the same generic
// response as for an unknown address.
func (r *Repository) GetUserByPendingEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + `
		FROM users
		WHERE pending_email_normalized = $1 AND pending_email IS NOT NULL AND deleted_at IS NULL
		ORDER BY created_at, id
		LIMIT 2`
	rows, err := r.pool.Query(ctx, query, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matched *models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		if matched != nil {
			return nil, ErrAmbiguousEmailClaim
		}
		matched = user
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return matched, nil
}

// promotePendingEmailOn applies the pending-claim promotion inside an existing
// transaction. It locks the user row, rejects claims that collide with an
// already-verified address, and atomically moves the pending claim into the
// verified email columns. The unique index on email_normalized is the final
// arbiter under concurrency; a unique violation is translated to
// ErrClaimConflict so callers never surface raw constraint errors.
func promotePendingEmailOn(ctx context.Context, tx pgx.Tx, userID, expectedNormalized string) error {
	var pendingEmail, pendingNormalized sql.NullString
	err := tx.QueryRow(ctx, `SELECT pending_email, pending_email_normalized FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&pendingEmail, &pendingNormalized)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if !pendingEmail.Valid || !pendingNormalized.Valid {
		return ErrNothingToPromote
	}
	if expectedNormalized != "" && pendingNormalized.String != expectedNormalized {
		return ErrTokenInvalid
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
