package groups

import (
	"context"
	"errors"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// Create inserts a group and, when userID is non-empty, its creator's
// membership in a single transaction.
func (r *Repository) Create(ctx context.Context, group *models.Group, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO groups (id, name, code, created_at) VALUES ($1, $2, $3, $4)`, group.ID, group.Name, group.Code, group.CreatedAt); err != nil {
		return err
	}
	if userID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3)`, group.ID, userID, group.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ByCode resolves a group by its invite code, returning nil when no group has
// the code.
func (r *Repository) ByCode(ctx context.Context, code string) (*models.Group, error) {
	return scanGroup(r.pool.QueryRow(ctx, `SELECT id, name, code, created_at FROM groups WHERE code = $1`, code))
}

// ByID resolves a group by its id, returning nil when it does not exist.
func (r *Repository) ByID(ctx context.Context, groupID string) (*models.Group, error) {
	return scanGroup(r.pool.QueryRow(ctx, `SELECT id, name, code, created_at FROM groups WHERE id = $1`, groupID))
}

// GroupPhoto returns the current photo of a group, or nil when the group has
// none.
func (r *Repository) GroupPhoto(ctx context.Context, groupID string) (*models.GroupPhoto, error) {
	return scanGroupPhoto(r.pool.QueryRow(ctx, `SELECT group_id, storage_key, mime_type, byte_size, created_at FROM group_photos WHERE group_id = $1`, groupID))
}

// SetGroupPhoto atomically points a group at a newly stored photo and returns
// the previous storage key so callers can retire the replaced object after the
// database commit succeeds.
func (r *Repository) SetGroupPhoto(ctx context.Context, photo *models.GroupPhoto) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM group_photos WHERE group_id = $1 FOR UPDATE`, photo.GroupID).Scan(&previousKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO group_photos (group_id, storage_key, mime_type, byte_size, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id) DO UPDATE SET storage_key = EXCLUDED.storage_key, mime_type = EXCLUDED.mime_type, byte_size = EXCLUDED.byte_size, created_at = EXCLUDED.created_at`, photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return previousKey, nil
}
