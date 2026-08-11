package groups

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// CreateInvite stores a new group invite whose token is only ever kept as a
// SHA-256 hash (invite.TokenHash). The active-invite cap per group and the
// per-creator daily creation cap are enforced inside one transaction that
// locks the group and creator rows, so concurrent creations cannot overshoot
// either limit across backend replicas.
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
	var membership int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2 FOR SHARE`, invite.GroupID, invite.CreatorUserID).Scan(&membership); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotMember
		}
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

// InviteByID resolves an invite by its id, or returns nil when it does not
// exist. It never exposes the token hash.
func (r *Repository) InviteByID(ctx context.Context, inviteID string) (*models.GroupInvite, error) {
	return scanGroupInviteMetadata(r.pool.QueryRow(ctx, `SELECT id, group_id, creator_user_id, created_at, expires_at, revoked_at FROM group_invites WHERE id = $1`, inviteID))
}

// ListInvitesByGroup returns every invite only while userID is a current group
// member. Token hashes are never selected, so management reads cannot carry
// bearer-derived material beyond the lookup path that needs it.
func (r *Repository) ListInvitesByGroup(ctx context.Context, groupID, userID string) ([]models.GroupInvite, error) {
	rows, err := r.pool.Query(ctx, `SELECT gi.id, gi.group_id, gi.creator_user_id, gi.created_at, gi.expires_at, gi.revoked_at
		FROM group_invites gi
		JOIN group_members gm ON gm.group_id = gi.group_id AND gm.user_id = $2
		WHERE gi.group_id = $1 ORDER BY gi.created_at DESC`, groupID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []models.GroupInvite{}
	for rows.Next() {
		var invite models.GroupInvite
		if err := rows.Scan(&invite.ID, &invite.GroupID, &invite.CreatorUserID, &invite.CreatedAt, &invite.ExpiresAt, &invite.RevokedAt); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// RevokeInvite marks an invite revoked unless it is already revoked. It
// returns true when this call performed the revocation and false when the
// invite does not exist or was already revoked.
func (r *Repository) RevokeInvite(ctx context.Context, inviteID, groupID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE group_invites gi SET revoked_at = now()
		WHERE gi.id = $1 AND gi.group_id = $2 AND gi.revoked_at IS NULL
		AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = gi.group_id AND gm.user_id = $3)`, inviteID, groupID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// JoinByInviteTokenHash atomically validates a live invite and adds userID to
// its group. Locking the invite row serializes joins with revocation, so a
// revocation cannot land between credential validation and membership grant.
// Replays by an existing member are intentionally idempotent.
func (r *Repository) JoinByInviteTokenHash(ctx context.Context, tokenHash, userID string, joinedAt time.Time) (*models.Group, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	group, err := scanGroup(tx.QueryRow(ctx, `SELECT g.id, g.name, g.code, g.created_at
		FROM group_invites gi JOIN groups g ON g.id = gi.group_id
		WHERE gi.token_hash = $1 AND gi.revoked_at IS NULL AND gi.expires_at > $2
		FOR UPDATE OF gi`, tokenHash, joinedAt))
	if err != nil || group == nil {
		return group, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3) ON CONFLICT (group_id, user_id) DO NOTHING`, group.ID, userID, joinedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return group, nil
}

// GroupPreviewByInviteTokenHash validates the live invite and returns its
// public group preview in one statement. Keeping validation and the read
// together prevents a revocation from landing between them.
func (r *Repository) GroupPreviewByInviteTokenHash(ctx context.Context, tokenHash string, previewedAt time.Time) (string, int, bool, error) {
	var name string
	var memberCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT g.name, (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id)
		FROM group_invites gi JOIN groups g ON g.id = gi.group_id
		WHERE gi.token_hash = $1 AND gi.revoked_at IS NULL AND gi.expires_at > $2`, tokenHash, previewedAt).Scan(&name, &memberCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}
	return name, memberCount, true, nil
}
