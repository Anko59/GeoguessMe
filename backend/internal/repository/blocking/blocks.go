package blocking

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"
)

var ErrUnavailable = errors.New("player unavailable")

type Repository struct{ pool database.Pool }

func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }

type Block struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
}

// Block is an idempotent, directed preference. Eligibility does not require the
// target to be currently visible: mutual blocks and retries must not reveal
// which player blocked first. Shared membership and public authorship are the
// same discoverability boundaries used by the profile and feed interfaces.
func (r *Repository) Block(ctx context.Context, owner, target string) error {
	if owner == target {
		return ErrUnavailable
	}
	tag, err := r.pool.Exec(ctx, `INSERT INTO user_blocks(blocker_id, blocked_id)
 SELECT $1, u.id FROM users u WHERE u.id=$2 AND u.deleted_at IS NULL AND (
 EXISTS (SELECT 1 FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2)
 OR EXISTS (SELECT 1 FROM group_members mine JOIN group_members theirs ON theirs.group_id=mine.group_id WHERE mine.user_id=$1 AND theirs.user_id=u.id)
 OR EXISTS (SELECT 1 FROM public_challenges p WHERE p.user_id=u.id AND p.audience='public'))
 ON CONFLICT (blocker_id, blocked_id) DO UPDATE SET created_at=user_blocks.created_at`, owner, target)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnavailable
	}
	return nil
}

// Unblock deliberately does not require a current shared group or a live target.
func (r *Repository) Unblock(ctx context.Context, owner, target string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`, owner, target)
	return err
}

func (r *Repository) List(ctx context.Context, owner string) ([]Block, error) {
	rows, err := r.pool.Query(ctx, `SELECT b.blocked_id,u.username,u.avatar,b.created_at
 FROM user_blocks b JOIN users u ON u.id=b.blocked_id AND u.deleted_at IS NULL
 WHERE b.blocker_id=$1 ORDER BY b.created_at DESC,b.blocked_id DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Block{}
	for rows.Next() {
		var item Block
		if err := rows.Scan(&item.UserID, &item.Username, &item.Avatar, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Blocked is symmetric and never caches authorizations. Query failures must be
// propagated by callers; they cannot be interpreted as permission to disclose.
func (r *Repository) Blocked(ctx context.Context, viewer, target string) (bool, error) {
	if viewer == target {
		return false, nil
	}
	var blocked bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_blocks WHERE
 (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1))`, viewer, target).Scan(&blocked)
	return blocked, err
}
