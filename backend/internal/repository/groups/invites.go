package groups

import (
	"context"
	"errors"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// CreateInvite stores a new group invite whose token is only ever kept as a
// SHA-256 hash (invite.TokenHash). The active-invite cap per group and the
// per-creator daily creation cap are enforced inside one transaction that
// locks the group and creator rows, so concurrent creations cannot overshoot
// either limit on the single backend replica.
func (r *Repository) CreateInvite(ctx context.Context, invite *models.GroupInvite, maxActivePerGroup, maxPerUserPerDay int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize creations for this group and creator so the two cap counts
	// below cannot race with a concurrent insert.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM groups WHERE id = $1 FOR UPDATE`, invite.GroupID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, invite.CreatorUserID); err != nil {
		return err
	}

	var active int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM group_invites WHERE group_id = $1 AND revoked_at IS NULL AND expires_at > now()`, invite.GroupID).Scan(&active); err != nil {
		return err
	}
	if active >= maxActivePerGroup {
		return ErrTooManyGroupInvites
	}

	var createdToday int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM group_invites WHERE creator_user_id = $1 AND created_at >= date_trunc('day', now())`, invite.CreatorUserID).Scan(&createdToday); err != nil {
		return err
	}
	if createdToday >= maxPerUserPerDay {
		return ErrTooManyUserInvites
	}

	if _, err := tx.Exec(ctx, `INSERT INTO group_invites (id, group_id, creator_user_id, token_hash, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		invite.ID, invite.GroupID, invite.CreatorUserID, invite.TokenHash, invite.CreatedAt, invite.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InviteByTokenHash resolves an invite by its hashed token, or returns nil
// when no invite carries that hash.
func (r *Repository) InviteByTokenHash(ctx context.Context, tokenHash string) (*models.GroupInvite, error) {
	return scanGroupInvite(r.pool.QueryRow(ctx, `SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE token_hash = $1`, tokenHash))
}

// InviteByID resolves an invite by its id, or returns nil when it does not
// exist. It never exposes the token hash.
func (r *Repository) InviteByID(ctx context.Context, inviteID string) (*models.GroupInvite, error) {
	return scanGroupInvite(r.pool.QueryRow(ctx, `SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE id = $1`, inviteID))
}

// ListInvitesByGroup returns every invite of a group. Token hashes are never
// selected, so list responses cannot leak the bearer value.
func (r *Repository) ListInvitesByGroup(ctx context.Context, groupID string) ([]models.GroupInvite, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE group_id = $1 ORDER BY created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []models.GroupInvite{}
	for rows.Next() {
		var invite models.GroupInvite
		if err := rows.Scan(&invite.ID, &invite.GroupID, &invite.CreatorUserID, &invite.TokenHash, &invite.CreatedAt, &invite.ExpiresAt, &invite.RevokedAt); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// RevokeInvite marks an invite revoked unless it is already revoked. It
// returns true when this call performed the revocation and false when the
// invite does not exist or was already revoked.
func (r *Repository) RevokeInvite(ctx context.Context, inviteID, groupID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE group_invites SET revoked_at = now() WHERE id = $1 AND group_id = $2 AND revoked_at IS NULL`, inviteID, groupID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// GroupPreview returns the public, non-sensitive preview data for an invite:
// the group name and its current member count. Callers must validate the
// invite (token hash match, unrevoked, unexpired) before calling this.
func (r *Repository) GroupPreview(ctx context.Context, groupID string) (string, int, error) {
	var name string
	var memberCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT g.name, (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id)
		FROM groups g WHERE g.id = $1`, groupID).Scan(&name, &memberCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrNotFound
		}
		return "", 0, err
	}
	return name, memberCount, nil
}
