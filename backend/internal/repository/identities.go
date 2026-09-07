package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrOIDCAccountLinkRequired prevents a new social identity from creating a
	// duplicate account when an existing unverified claim uses the same email.
	ErrOIDCAccountLinkRequired = errors.New("existing account must be linked explicitly")
	// ErrOIDCIdentityConflict means either side of a requested link already
	// belongs to a different account.
	ErrOIDCIdentityConflict = errors.New("OIDC identity is already linked")
	// ErrOIDCLinkIntentInvalid covers missing, expired, and consumed intents.
	ErrOIDCLinkIntentInvalid = errors.New("OIDC link intent is invalid")
	// ErrOIDCUsernameRequired keeps identity verification separate from the
	// public game profile: every genuinely new player chooses their username.
	ErrOIDCUsernameRequired = errors.New("username is required for a new OIDC account")
)

// OIDCIdentity is the verified, minimal identity projection accepted by the
// persistence layer. Issuer + Subject is the durable key; email is only a
// verified bootstrap/linking signal and never replaces that key.
type OIDCIdentity struct {
	Issuer  string
	Subject string
	Email   string
}

// LegacyIdentityInventory contains only aggregate migration categories plus
// the verified addresses eligible for Keycloak pre-provisioning. Callers must
// never print VerifiedEmails in routine logs or release evidence.
type LegacyIdentityInventory struct {
	Total          int
	Linked         int
	Verified       int
	Pending        int
	Missing        int
	VerifiedEmails []string
}

// LegacyIdentityMigrationInventory classifies active accounts that still
// retain a legacy password. Only an unlinked, application-verified email is
// safe to pre-provision; pending and missing addresses require the explicit
// legacy-authenticated linking flow.
func (r *Repository) LegacyIdentityMigrationInventory(ctx context.Context) (LegacyIdentityInventory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.email, u.email_verified_at IS NOT NULL, u.pending_email,
		       EXISTS (SELECT 1 FROM user_identities AS ui WHERE ui.user_id = u.id)
		FROM users AS u
		WHERE u.deleted_at IS NULL AND u.legacy_password_enabled = TRUE
		ORDER BY u.created_at, u.id`)
	if err != nil {
		return LegacyIdentityInventory{}, err
	}
	defer rows.Close()
	inventory := LegacyIdentityInventory{}
	for rows.Next() {
		var email, pending sql.NullString
		var verified, linked bool
		if err := rows.Scan(&email, &verified, &pending, &linked); err != nil {
			return LegacyIdentityInventory{}, err
		}
		inventory.Total++
		switch {
		case linked:
			inventory.Linked++
		case verified && email.Valid && strings.TrimSpace(email.String) != "":
			inventory.Verified++
			inventory.VerifiedEmails = append(inventory.VerifiedEmails, strings.ToLower(strings.TrimSpace(email.String)))
		case pending.Valid && strings.TrimSpace(pending.String) != "":
			inventory.Pending++
		default:
			inventory.Missing++
		}
	}
	return inventory, rows.Err()
}

// OIDCIdentitiesByUserID returns the upstream subjects that must be removed
// before an OIDC-linked application account can be deleted.
func (r *Repository) OIDCIdentitiesByUserID(ctx context.Context, userID string) ([]OIDCIdentity, error) {
	rows, err := r.pool.Query(ctx, `SELECT issuer, subject, email_at_link FROM user_identities WHERE user_id = $1 ORDER BY issuer, subject`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := make([]OIDCIdentity, 0)
	for rows.Next() {
		var identity OIDCIdentity
		if err := rows.Scan(&identity.Issuer, &identity.Subject, &identity.Email); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

// CreateOIDCLinkIntent records a short-lived, single-use proof that a legacy
// account requested an identity link. Only the caller-provided token digest is
// persisted.
func (r *Repository) CreateOIDCLinkIntent(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM oidc_link_intents WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO oidc_link_intents(token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, tokenHash, userID, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveOIDCIdentity returns an already-linked account, safely attaches a
// first login to an exact verified-email match, or creates a new account. A
// pending/unverified email match is deliberately never trusted for automatic
// linking and instead requires a legacy-authenticated link intent.
func (r *Repository) ResolveOIDCIdentity(ctx context.Context, identity OIDCIdentity, userID, username, avatar string, now time.Time) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockOIDCIdentity(ctx, tx, identity); err != nil {
		return nil, err
	}

	linkedUserID, err := identityOwner(ctx, tx, identity)
	if err != nil {
		return nil, err
	}
	if linkedUserID != "" {
		if _, err := tx.Exec(ctx, `UPDATE user_identities SET last_login_at = $1 WHERE issuer = $2 AND subject = $3`, now, identity.Issuer, identity.Subject); err != nil {
			return nil, err
		}
		return commitLoadedUser(ctx, tx, linkedUserID)
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(identity.Email))
	// Serialize first logins that present the same verified email as well as
	// those that present the same subject. This keeps email auto-link/create
	// decisions deterministic under concurrent callbacks.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "email:"+normalizedEmail); err != nil {
		return nil, err
	}
	var existingUserID string
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE email_normalized = $1 AND email_verified_at IS NOT NULL AND deleted_at IS NULL`, normalizedEmail).Scan(&existingUserID)
	switch {
	case err == nil:
		if err := insertOIDCIdentity(ctx, tx, identity, existingUserID, now); err != nil {
			return nil, err
		}
		if err := completeLegacyMigration(ctx, tx, existingUserID, now); err != nil {
			return nil, err
		}
		return commitLoadedUser(ctx, tx, existingUserID)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	var pendingMatch bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE pending_email_normalized = $1 AND deleted_at IS NULL)`, normalizedEmail).Scan(&pendingMatch); err != nil {
		return nil, err
	}
	if pendingMatch {
		return nil, ErrOIDCAccountLinkRequired
	}
	if strings.TrimSpace(username) == "" {
		return nil, ErrOIDCUsernameRequired
	}

	inserted, err := insertOIDCUser(ctx, tx, userID, username, identity.Email, normalizedEmail, avatar, now)
	if err != nil {
		return nil, err
	}
	if !inserted {
		return nil, ErrUsernameConflict
	}
	if err := insertOIDCIdentity(ctx, tx, identity, userID, now); err != nil {
		return nil, err
	}
	return commitLoadedUser(ctx, tx, userID)
}

// LinkOIDCIdentity consumes an authenticated link intent and binds the
// verified Keycloak subject to that exact application account.
func (r *Repository) LinkOIDCIdentity(ctx context.Context, tokenHash string, identity OIDCIdentity, now time.Time) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockOIDCIdentity(ctx, tx, identity); err != nil {
		return nil, err
	}
	var userID string
	err = tx.QueryRow(ctx, `UPDATE oidc_link_intents SET used_at = $1 WHERE token_hash = $2 AND used_at IS NULL AND expires_at > $1 RETURNING user_id`, now, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOIDCLinkIntentInvalid
	}
	if err != nil {
		return nil, err
	}
	owner, err := identityOwner(ctx, tx, identity)
	if err != nil {
		return nil, err
	}
	if owner != "" && owner != userID {
		return nil, ErrOIDCIdentityConflict
	}
	if owner == "" {
		if err := insertOIDCIdentity(ctx, tx, identity, userID, now); err != nil {
			return nil, err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE user_identities SET last_login_at = $1 WHERE issuer = $2 AND subject = $3`, now, identity.Issuer, identity.Subject); err != nil {
		return nil, err
	}
	if err := completeLegacyMigration(ctx, tx, userID, now); err != nil {
		return nil, err
	}
	return commitLoadedUser(ctx, tx, userID)
}

func completeLegacyMigration(ctx context.Context, tx pgx.Tx, userID string, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE users SET auth_version = auth_version + 1, updated_at = $2 WHERE id = $1`, userID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, now); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM websocket_tickets WHERE user_id = $1`, userID)
	return err
}

func lockOIDCIdentity(ctx context.Context, tx pgx.Tx, identity OIDCIdentity) error {
	// PostgreSQL text cannot contain NUL bytes. A length-prefixed issuer keeps
	// the composite key unambiguous without passing an invalid UTF-8 sequence.
	lockKey := fmt.Sprintf("identity:%d:%s%s", len(identity.Issuer), identity.Issuer, identity.Subject)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey)
	return err
}

func identityOwner(ctx context.Context, tx pgx.Tx, identity OIDCIdentity) (string, error) {
	var userID string
	err := tx.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE issuer = $1 AND subject = $2`, identity.Issuer, identity.Subject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

func insertOIDCIdentity(ctx context.Context, tx pgx.Tx, identity OIDCIdentity, userID string, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO user_identities(issuer, subject, user_id, email_at_link, linked_at, last_login_at) VALUES ($1, $2, $3, $4, $5, $5)`, identity.Issuer, identity.Subject, userID, identity.Email, now)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrOIDCIdentityConflict
	}
	return err
}

func insertOIDCUser(ctx context.Context, tx pgx.Tx, userID, username, email, normalizedEmail, avatar string, now time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, email_normalized, email_verified_at, pending_email, pending_email_normalized, password, legacy_password_enabled, avatar, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NULL, NULL, '!', FALSE, $6, $5, $5) ON CONFLICT (username) DO NOTHING`, userID, username, email, normalizedEmail, now, avatar)
	return err == nil && tag.RowsAffected() == 1, err
}

func commitLoadedUser(ctx context.Context, tx pgx.Tx, userID string) (*models.User, error) {
	user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
	if err != nil || user == nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return user, nil
}
