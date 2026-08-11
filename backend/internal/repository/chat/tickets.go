package chat

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateWebSocketTicket persists a one-time realtime chat ticket scoped to a
// user and group. authVersion is the issuing user's current users.auth_version
// so consumption can reject tickets minted before any credential revocation.
func (r *Repository) CreateWebSocketTicket(ctx context.Context, id, userID, groupID, tokenHash string, authVersion int, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO websocket_tickets(id, user_id, group_id, token_hash, auth_version, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`, id, userID, groupID, tokenHash, authVersion, expiresAt)
	return err
}

// ConsumeWebSocketTicket atomically marks a ticket used and returns its user
// id. A consumed, missing, or expired ticket yields an empty user id so the
// caller can reject the upgrade. Consumption additionally requires the user to
// be active, the ticket's auth_version to match the user's current value, and
// the user to remain a member of the group; when any of those checks fail the
// ticket is left unused so it fails identically on any retry.
func (r *Repository) ConsumeWebSocketTicket(ctx context.Context, tokenHash, groupID string) (userID string, err error) {
	err = r.pool.QueryRow(ctx, `UPDATE websocket_tickets t SET used_at = CURRENT_TIMESTAMP FROM users u WHERE t.token_hash = $1 AND t.group_id = $2 AND t.used_at IS NULL AND t.expires_at > CURRENT_TIMESTAMP AND t.user_id = u.id AND u.deleted_at IS NULL AND t.auth_version = u.auth_version AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.user_id = t.user_id AND gm.group_id = t.group_id) RETURNING t.user_id`, tokenHash, groupID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// DeleteWebSocketTickets removes every outstanding ticket for a user. It backs
// credential-revocation paths so a stale ticket can never be consumed.
func (r *Repository) DeleteWebSocketTickets(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM websocket_tickets WHERE user_id = $1`, userID)
	return err
}

// UserAuthVersion reports the user's current auth_version so a newly issued
// ticket can be bound to the exact version that authenticated the request. A
// missing or deleted account yields version 0, which consumption rejects
// against any real user version.
func (r *Repository) UserAuthVersion(ctx context.Context, userID string) (int, error) {
	var version int
	err := r.pool.QueryRow(ctx, `SELECT auth_version FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return version, err
}
