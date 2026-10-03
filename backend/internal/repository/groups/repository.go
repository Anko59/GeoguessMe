package groups

import (
	"context"
	"geoguessme/internal/database"
	"geoguessme/internal/models"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Repository is the gameplay persistence collection: group membership and
// notification preferences, group and challenge data, and leaderboard/ranking
// queries. PR 6 moves the group, challenge, guess, delivery, and leaderboard
// slices off the database.DB package global onto methods bound to an injected
// pool, mirroring the internal/repository/chat package from PR 5.
//
// The three responsibilities from the refactor roadmap live in separate files:
// membership/preferences (membership.go), group/challenge data (groups.go and
// challenges.go), and ranking/leaderboard queries (leaderboard.go). Row
// scanning and column definitions have one owner: scan.go.
//
// Instances are independent: two Repositories built on different pools never
// share state.
type Repository struct {
	pool database.Pool
}

// NewRepository returns a Repository bound to the given pool.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool}
}

// photoVisibility hides challenge content, not group membership or rankings.
const photoVisibility = `NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
	(b.blocker_id=$2 AND b.blocked_id=photos.user_id)
	OR (b.blocker_id=photos.user_id AND b.blocked_id=$2))`

const photoColumns = `id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, hide_location, created_at, expires_at, retention_at`

type photoQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func visiblePhoto(ctx context.Context, q photoQuerier, id, viewerID string, lock bool) (*models.Photo, error) {
	query := `SELECT ` + photoColumns + ` FROM photos WHERE id = $1 AND ` + photoVisibility
	if lock {
		query += ` FOR UPDATE`
	}
	return scanPhoto(q.QueryRow(ctx, query, id, viewerID))
}

// PhotoForViewer is the authenticated photo lookup. Membership remains the
// caller's responsibility, as it is for Photo; blocking is never bypassed.
func (r *Repository) PhotoForViewer(ctx context.Context, id, viewerID string) (*models.Photo, error) {
	return visiblePhoto(ctx, r.pool, id, viewerID, false)
}

func viewerGuessQuery(query, photoID string, viewers ...string) (string, []any) {
	args := []any{photoID}
	if len(viewers) > 0 && viewers[0] != "" {
		query = strings.Replace(query, " ORDER BY", ` AND NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
			(b.blocker_id=$2 AND b.blocked_id=g.user_id) OR (b.blocker_id=g.user_id AND b.blocked_id=$2)) ORDER BY`, 1)
		args = append(args, viewers[0])
	}
	return query, args
}

// GuessesForPhotoForViewer hides blocked contributors' guess content without
// changing stored scores, group membership, or leaderboard calculations.
func (r *Repository) GuessesForPhotoForViewer(ctx context.Context, photoID, viewerID string) ([]GuessWithUser, error) {
	return r.GuessesForPhoto(ctx, photoID, viewerID)
}

const inboxMessageVisibility = `NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
	(b.blocker_id=$1 AND b.blocked_id=m.user_id) OR (b.blocker_id=m.user_id AND b.blocked_id=$1))`
