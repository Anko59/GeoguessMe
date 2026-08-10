package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/repository/groups"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	userColumns = "id, username, email, password, avatar, COALESCE(email_verified_at, NULL), auth_version, created_at, updated_at"
)

// CreateUser inserts a new account.
func (r *Repository) CreateUser(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, username, email, email_normalized, password, avatar, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`
	_, err := r.pool.Exec(ctx, query, user.ID, user.Username, user.Email, strings.ToLower(strings.TrimSpace(user.Email)), user.Password, user.Avatar, user.CreatedAt)
	return err
}

// GetUserByUsername resolves an account by its unique username.
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE username = $1 AND deleted_at IS NULL`
	return scanUser(r.pool.QueryRow(ctx, query, username))
}

// GetUserByEmail resolves an account by its normalized email address.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email_normalized = $1 AND deleted_at IS NULL`
	return scanUser(r.pool.QueryRow(ctx, query, strings.ToLower(strings.TrimSpace(email))))
}

// GetUserByID resolves an account by id.
func (r *Repository) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1 AND deleted_at IS NULL`
	return scanUser(r.pool.QueryRow(ctx, query, userID))
}

type UserScoreStats struct {
	TotalPoints  int
	GuessCount   int
	AverageScore float64
}

// GetUserScoreStats aggregates a player's guess statistics.
func (r *Repository) GetUserScoreStats(ctx context.Context, userID string) (UserScoreStats, error) {
	var stats UserScoreStats
	var totalPoints, guessCount int64
	var averageScore float64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(score), 0), COUNT(*), COALESCE(AVG(score), 0)
		FROM guesses
		WHERE user_id = $1`, userID).Scan(&totalPoints, &guessCount, &averageScore)
	if err != nil {
		return UserScoreStats{}, err
	}
	stats.TotalPoints = int(totalPoints)
	stats.GuessCount = int(guessCount)
	stats.AverageScore = averageScore
	return stats, nil
}

// GetGlobalRank calculates a stable snapshot in one query. Deleted accounts
// are excluded from both the requested rank and the population.
func (r *Repository) GetGlobalRank(ctx context.Context, userID string) (groups.GlobalRankStats, error) {
	var rank, totalPlayers int64
	err := r.pool.QueryRow(ctx, `
		WITH totals AS (
			SELECT g.user_id, SUM(g.score) AS total_points
			FROM guesses g
			JOIN users u ON u.id = g.user_id AND u.deleted_at IS NULL
			GROUP BY g.user_id
		), ranked AS (
			SELECT user_id, RANK() OVER (ORDER BY total_points DESC) AS global_rank
			FROM totals
		)
		SELECT COALESCE(MAX(global_rank) FILTER (WHERE user_id = $1), 0), COUNT(*)
		FROM ranked`, userID).Scan(&rank, &totalPlayers)
	if err != nil {
		return groups.GlobalRankStats{}, err
	}
	return groups.GlobalRankStats{Rank: int(rank), TotalPlayers: int(totalPlayers)}, nil
}

// AuthStatus summarises what protected middleware must check on every request:
// whether the account still exists (is active) and its current auth version.
type AuthStatus struct {
	Active      bool
	AuthVersion int
}

// GetUserAuthStatus reports the account's activity and auth version for
// protected-route middleware.
func (r *Repository) GetUserAuthStatus(ctx context.Context, userID string) (AuthStatus, error) {
	var version int
	err := r.pool.QueryRow(ctx, `SELECT auth_version FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&version)
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

// CreateRefreshSession persists a refresh session with its hashed token.
func (r *Repository) CreateRefreshSession(ctx context.Context, session RefreshSession, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO refresh_sessions(id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, session.ID, session.UserID, tokenHash, session.ExpiresAt)
	return err
}

// RotateRefreshSession atomically retires the presented session, verifies the
// account is still active, and installs the replacement session. Returning a
// non-nil user signals success; a nil user signals the presented token was
// invalid, expired, or already used.
func (r *Repository) RotateRefreshSession(ctx context.Context, presentedHash, replacementID, replacementHash string, replacementExpiresAt, now time.Time) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
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

// RevokeRefreshSessionByHash revokes an active refresh session by its token
// hash, as used by explicit logout.
func (r *Repository) RevokeRefreshSessionByHash(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return err
}

// UserIDByRefreshHash resolves the owner of a refresh session for logout-all.
func (r *Repository) UserIDByRefreshHash(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM refresh_sessions WHERE token_hash = $1`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// BumpAuthVersion invalidates every outstanding access token for a user by
// changing the value their claims must match. Used by explicit "logout all".
func (r *Repository) BumpAuthVersion(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID)
	return err
}

// RevokeAllRefreshSessions revokes every active refresh session for a user.
func (r *Repository) RevokeAllRefreshSessions(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

// RevokeAllCredentials atomically invalidates every credential for a user:
// bumps the auth version (invalidating outstanding access tokens), revokes
// every refresh session, and deletes outstanding WebSocket tickets. It backs
// explicit "logout all" so a global revocation can never leave a partial
// revocation behind.
func (r *Repository) RevokeAllCredentials(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE users SET auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM websocket_tickets WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InsertOneTimeToken stores a fresh email-verification or password-reset
// token, replacing any unused token of the same kind for the user.
func (r *Repository) InsertOneTimeToken(ctx context.Context, table, id, userID, hash string, expiresAt time.Time) error {
	if table != "email_verification_tokens" && table != "password_reset_tokens" {
		return errors.New("invalid one-time token table")
	}
	tx, err := r.pool.Begin(ctx)
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
func (r *Repository) VerifyEmailTransaction(ctx context.Context, tokenHash string) error {
	tx, err := r.pool.Begin(ctx)
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
// ResetPasswordTransaction consumes a one-time reset token and installs a new
// password, returning the owning user so callers can close that user's live
// sockets. Token consumption, password update, auth-version bump, session
// revocation, and WebSocket-ticket revocation happen atomically.
func (r *Repository) ResetPasswordTransaction(ctx context.Context, tokenHash, passwordHash string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx, `UPDATE password_reset_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrTokenInvalid
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password = $1, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, passwordHash, userID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return "", err
	}
	// F-03: outstanding WebSocket tickets are revoked atomically with the reset
	// so a ticket minted before the reset can never open a live socket.
	if _, err := tx.Exec(ctx, `DELETE FROM websocket_tickets WHERE user_id = $1`, userID); err != nil {
		return "", err
	}
	return userID, tx.Commit(ctx)
}

// ErrTokenInvalid is returned when a one-time token is absent, expired, or
// already consumed.
var ErrTokenInvalid = errors.New("token is invalid or expired")

// SetUserAvatar stores the avatar marker for a user without touching any
// other profile field. It backs the authenticated avatar upload endpoint.
func (r *Repository) SetUserAvatar(ctx context.Context, userID, avatar string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET avatar = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND deleted_at IS NULL`, avatar, userID)
	return err
}

// UpdateProfile changes the public account fields. Changing the email address
// clears verification so the new address must be verified independently.
func (r *Repository) UpdateProfile(ctx context.Context, userID, username, email, avatar string) (*models.User, error) {
	_, err := r.pool.Exec(ctx, `UPDATE users SET username = $1, email = $2, email_normalized = $3, avatar = $4, email_verified_at = CASE WHEN email_normalized <> $3 THEN NULL ELSE email_verified_at END, updated_at = CURRENT_TIMESTAMP WHERE id = $5 AND deleted_at IS NULL`, username, email, strings.ToLower(strings.TrimSpace(email)), avatar, userID)
	if err != nil {
		return nil, err
	}
	return r.GetUserByID(ctx, userID)
}

// ChangePassword updates the password and invalidates every existing session
// atomically, including the session used for the request.
func (r *Repository) ChangePassword(ctx context.Context, userID, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE users SET password = $1, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND deleted_at IS NULL`, passwordHash, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	// F-03: outstanding WebSocket tickets are revoked atomically with the
	// password change so a stale ticket can never open a live socket.
	if _, err := tx.Exec(ctx, `DELETE FROM websocket_tickets WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteUserCascade removes the account and every related row, and enqueues
// durable deletion jobs for the media the account authored so object storage
// can never be orphaned. The returned keys are for observability only.
func (r *Repository) DeleteUserCascade(ctx context.Context, userID string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
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
		if _, err := tx.Exec(ctx, `INSERT INTO media_deletion_jobs(id, storage_key, source) VALUES ($1, $2, 'account') ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING`, uuid.NewString(), key); err != nil {
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

// CleanupAuthTokens deletes expired one-time and refresh tokens plus expired
// WebSocket tickets. It backs the periodic cleanup worker.
func (r *Repository) CleanupAuthTokens(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_sessions WHERE expires_at < CURRENT_TIMESTAMP OR revoked_at < CURRENT_TIMESTAMP - interval '30 days'; DELETE FROM email_verification_tokens WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'; DELETE FROM password_reset_tokens WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'; DELETE FROM websocket_tickets WHERE expires_at < CURRENT_TIMESTAMP OR used_at < CURRENT_TIMESTAMP - interval '1 day'`)
	return err
}
