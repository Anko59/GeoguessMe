package feed

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("public challenge not found")
var ErrForbidden = errors.New("action not allowed")

type Repository struct{ pool database.Pool }

func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }

type NewChallenge struct {
	ID, UserID, Caption, StorageKey, MIMEType string
	Preview                                   []byte
	Lat, Long                                 float64
	CreatedAt                                 time.Time
}

func (r *Repository) Create(ctx context.Context, p NewChallenge) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO public_challenges
		(id,user_id,caption,storage_key,mime_type,preview,lat,long,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.ID, p.UserID, p.Caption, p.StorageKey, p.MIMEType, p.Preview, p.Lat, p.Long, p.CreatedAt)
	return err
}

const selectPost = `SELECT p.id, p.user_id, u.username, p.caption, p.created_at,
	p.user_id = $1, EXISTS (SELECT 1 FROM public_guesses g WHERE g.challenge_id=p.id AND g.user_id=$1),
	(SELECT count(*) FROM public_reactions r WHERE r.challenge_id=p.id),
	EXISTS (SELECT 1 FROM public_reactions r WHERE r.challenge_id=p.id AND r.user_id=$1),
	(SELECT count(*) FROM public_comments c WHERE c.challenge_id=p.id)
	FROM public_challenges p JOIN users u ON u.id=p.user_id `

func scanPost(row interface{ Scan(...any) error }) (models.PublicChallenge, error) {
	var p models.PublicChallenge
	err := row.Scan(&p.ID, &p.UserID, &p.Username, &p.Caption, &p.CreatedAt, &p.IsOwner, &p.Resolved, &p.ReactionCount, &p.Reacted, &p.CommentCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (r *Repository) Get(ctx context.Context, id, viewer string) (models.PublicChallenge, error) {
	return scanPost(r.pool.QueryRow(ctx, selectPost+`WHERE p.id=$2`, viewer, id))
}

func (r *Repository) List(ctx context.Context, viewer string, cursor Cursor, limit int) (models.PublicFeedPage, error) {
	page := models.PublicFeedPage{Items: []models.PublicChallenge{}}
	query := selectPost
	args := []any{viewer, limit + 1}
	if cursor.ID != "" {
		// A direct seek predicate remains indexable with prepared generic plans.
		query += `WHERE (p.created_at,p.id)<($3,$4) `
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	rows, err := r.pool.Query(ctx, query+`ORDER BY p.created_at DESC,p.id DESC LIMIT $2`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, p)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// The delete trigger also covers account cascades and atomically records the
// storage deletion obligation. Only the author can remove a public post.
func (r *Repository) Delete(ctx context.Context, id, viewer string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM public_challenges WHERE id=$1 AND user_id=$2`, id, viewer)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type Media struct {
	StorageKey, MIMEType string
	Preview              []byte
	Revealed             bool
}

func (r *Repository) Media(ctx context.Context, id, viewer string) (Media, error) {
	var media Media
	err := r.pool.QueryRow(ctx, `SELECT storage_key,mime_type,preview,
		user_id=$2 OR EXISTS (SELECT 1 FROM public_guesses WHERE challenge_id=$1 AND user_id=$2)
		FROM public_challenges WHERE id=$1`, id, viewer).Scan(&media.StorageKey, &media.MIMEType, &media.Preview, &media.Revealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return media, ErrNotFound
	}
	return media, err
}
