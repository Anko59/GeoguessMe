package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	userColumns = "id, username, email, password, avatar, COALESCE(email_verified_at, NULL), auth_version, created_at, updated_at"
)

func CreateUser(user *models.User) error {
	return CreateUserContext(context.Background(), user)
}

func CreateUserContext(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, username, email, email_normalized, password, avatar, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`
	_, err := database.DB.Exec(ctx, query, user.ID, user.Username, user.Email, strings.ToLower(strings.TrimSpace(user.Email)), user.Password, user.Avatar, user.CreatedAt)
	return err
}

func GetUserByUsername(username string) (*models.User, error) {
	return GetUserByUsernameContext(context.Background(), username)
}

func GetUserByUsernameContext(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE username = $1 AND deleted_at IS NULL`
	return scanUser(database.DB.QueryRow(ctx, query, username))
}

func GetUserByEmail(email string) (*models.User, error) {
	return GetUserByEmailContext(context.Background(), email)
}

func GetUserByEmailContext(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email_normalized = $1 AND deleted_at IS NULL`
	return scanUser(database.DB.QueryRow(ctx, query, strings.ToLower(strings.TrimSpace(email))))
}

func GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1 AND deleted_at IS NULL`
	return scanUser(database.DB.QueryRow(ctx, query, userID))
}

// AuthStatus summarises what protected middleware must check on every request:
// whether the account still exists (is active) and its current auth version.
type AuthStatus struct {
	Active      bool
	AuthVersion int
}

func GetUserAuthStatus(ctx context.Context, userID string) (AuthStatus, error) {
	var version int
	err := database.DB.QueryRow(ctx, `SELECT auth_version FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthStatus{Active: false}, nil
	}
	if err != nil {
		return AuthStatus{}, err
	}
	return AuthStatus{Active: true, AuthVersion: version}, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(row rowScanner) (*models.User, error) {
	var user models.User
	var verified *time.Time
	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.Avatar, &verified, &user.AuthVersion, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user.EmailVerifiedAt = verified
	return &user, nil
}

type RefreshSession struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

func CreateRefreshSession(ctx context.Context, session RefreshSession, tokenHash string) error {
	_, err := database.DB.Exec(ctx, `INSERT INTO refresh_sessions(id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, session.ID, session.UserID, tokenHash, session.ExpiresAt)
	return err
}

// RotateRefreshSession atomically retires the presented session, verifies the
// account is still active, and installs the replacement session. Returning a
// non-nil user signals success; a nil user signals the presented token was
// invalid, expired, or already used.
func RotateRefreshSession(ctx context.Context, presentedHash, replacementID, replacementHash string, replacementExpiresAt, now time.Time) (*models.User, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx, `UPDATE refresh_sessions SET revoked_at = $1, last_used_at = $1 WHERE token_hash = $2 AND revoked_at IS NULL AND expires_at > $1 RETURNING user_id`, now, presentedHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
	if err != nil {
		return nil, err
	}
	if user == nil {
		// Account was deleted between token issue and rotation.
		return nil, nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO refresh_sessions(id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, replacementID, userID, replacementHash, replacementExpiresAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return user, nil
}

func RevokeRefreshSession(ctx context.Context, sessionID string) error {
	_, err := database.DB.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = $1`, sessionID)
	return err
}

func RevokeRefreshSessionByHash(ctx context.Context, tokenHash string) error {
	_, err := database.DB.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return err
}

// UserIDByRefreshHash resolves the owner of a refresh session for logout-all.
func UserIDByRefreshHash(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := database.DB.QueryRow(ctx, `SELECT user_id FROM refresh_sessions WHERE token_hash = $1`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// BumpAuthVersion invalidates every outstanding access token for a user by
// changing the value their claims must match. Used by explicit "logout all".
func BumpAuthVersion(ctx context.Context, userID string) error {
	_, err := database.DB.Exec(ctx, `UPDATE users SET auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID)
	return err
}

func RevokeAllRefreshSessions(ctx context.Context, userID string) error {
	_, err := database.DB.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func InsertOneTimeToken(ctx context.Context, table, id, userID, hash string, expiresAt time.Time) error {
	if table != "email_verification_tokens" && table != "password_reset_tokens" {
		return errors.New("invalid one-time token table")
	}
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE user_id = $1 AND used_at IS NULL", userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO "+table+"(id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)", id, userID, hash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// VerifyEmailTransaction consumes a verification token and marks the account
// verified in a single transaction so a crash cannot consume the token without
// updating the account.
func VerifyEmailTransaction(ctx context.Context, tokenHash string) error {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx, `UPDATE email_verification_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTokenInvalid
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET email_verified_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResetPasswordTransaction consumes a reset token, updates the password hash,
// bumps the auth version (invalidating outstanding access tokens), and revokes
// every refresh session — all atomically.
func ResetPasswordTransaction(ctx context.Context, tokenHash, passwordHash string) error {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx, `UPDATE password_reset_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTokenInvalid
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password = $1, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, passwordHash, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ErrTokenInvalid is returned when a one-time token is absent, expired, or
// already consumed.
var ErrTokenInvalid = errors.New("token is invalid or expired")

func MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := database.DB.Exec(ctx, `UPDATE users SET email_verified_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID)
	return err
}

func UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := database.DB.Exec(ctx, `UPDATE users SET password = $1, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, passwordHash, userID)
	return err
}

// DeleteUserCascade removes the account and every related row, and enqueues
// durable deletion jobs for the media the account authored so object storage
// can never be orphaned. The returned keys are for observability only.
func DeleteUserCascade(ctx context.Context, userID string) ([]string, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT storage_key FROM photos WHERE user_id = $1 AND storage_key IS NOT NULL`, userID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, key := range keys {
		if _, err := tx.Exec(ctx, `INSERT INTO media_deletion_jobs(id, storage_key, source) VALUES ($1, $2, 'account')`, uuid.NewString(), key); err != nil {
			return nil, err
		}
	}

	for _, table := range []string{"refresh_sessions", "email_verification_tokens", "password_reset_tokens", "websocket_tickets"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE user_id = $1", userID); err != nil {
			return nil, err
		}
	}
	// Removing the user cascades photos, messages, guesses, challenge_views,
	// and group_members (all ON DELETE CASCADE). Username/email uniqueness is
	// released so the identity can be reused.
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return keys, nil
}

func CleanupAuthTokens(ctx context.Context) error {
	_, err := database.DB.Exec(ctx, `DELETE FROM refresh_sessions WHERE expires_at < CURRENT_TIMESTAMP OR revoked_at < CURRENT_TIMESTAMP - interval '30 days'; DELETE FROM email_verification_tokens WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'; DELETE FROM password_reset_tokens WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'; DELETE FROM websocket_tickets WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'`)
	return err
}
