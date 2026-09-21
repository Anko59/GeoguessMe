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
var ErrProfileNotFound = errors.New("feed profile not found")
var ErrViewNotFinished = errors.New("viewing window is still open")
var ErrGuessTimeExpired = errors.New("guess window expired")
var ErrMediaExpired = errors.New("media viewing window expired")
var ErrOwnChallenge = errors.New("cannot use own challenge")
var ErrInvalidCoordinate = errors.New("invalid coordinate")
var ErrConflict = errors.New("public challenge id already belongs to another user")

type Repository struct{ pool database.Pool }

func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }

type NewChallenge struct {
	ID, UserID, Caption, StorageKey, MIMEType, ContentDigest string
	Audience                                                 string
	PublicationToken                                         string
	GroupIDs                                                 []string
	Preview                                                  []byte
	Lat, Long                                                float64
	ByteSize                                                 int64
	HideLocation                                             bool
	CreatedAt                                                time.Time
}

type PublicationResolution uint8

const (
	PublicationNotCommitted PublicationResolution = iota
	PublicationCommitted
	PublicationAlreadyCommitted
	PublicationConflict
)

// PublicationReservation serializes one idempotent feed publication from the
// reservation check through the object-store fan-out and the database commit.
// New reservations retain their transaction until Create or Rollback, so a
// concurrent retry cannot write the same canonical storage keys first.
type PublicationReservation interface {
	Existing() bool
	GroupIDs() []string
	Create(context.Context, NewChallenge, []*models.Photo) error
	Resolve(context.Context) (PublicationResolution, error)
	Rollback(context.Context) error
}

func (r *Repository) Create(ctx context.Context, p NewChallenge) error {
	if p.Audience == "" {
		p.Audience = "public"
	}
	if p.Audience != "public" && p.Audience != "friends" {
		return ErrForbidden
	}
	insert := `INSERT INTO public_challenges
		(id,user_id,caption,audience,storage_key,mime_type,preview,lat,long,hide_location,byte_size,content_digest,publication_token,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	if len(p.GroupIDs) == 0 {
		_, err := r.pool.Exec(ctx, insert, p.ID, p.UserID, p.Caption, p.Audience, p.StorageKey, p.MIMEType, p.Preview, p.Lat, p.Long, p.HideLocation, p.ByteSize, p.ContentDigest, p.PublicationToken, p.CreatedAt)
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, insert, p.ID, p.UserID, p.Caption, p.Audience, p.StorageKey, p.MIMEType, p.Preview, p.Lat, p.Long, p.HideLocation, p.ByteSize, p.ContentDigest, p.PublicationToken, p.CreatedAt); err != nil {
		return err
	}
	var memberCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM group_members WHERE user_id=$1 AND group_id=ANY($2::text[])`, p.UserID, p.GroupIDs).Scan(&memberCount); err != nil {
		return err
	}
	if memberCount != len(p.GroupIDs) {
		return ErrForbidden
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public_challenge_groups(challenge_id,group_id) SELECT $1, unnest($2::text[])`, p.ID, p.GroupIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const selectPost = `SELECT p.id, p.user_id, u.username, u.avatar, p.caption, p.created_at,
	p.audience, p.user_id = $1, EXISTS (SELECT 1 FROM public_guesses g WHERE g.challenge_id=p.id AND g.user_id=$1),
	(SELECT count(*) FROM public_reactions r WHERE r.challenge_id=p.id),
	EXISTS (SELECT 1 FROM public_reactions r WHERE r.challenge_id=p.id AND r.user_id=$1),
	(SELECT count(*) FROM public_comments c WHERE c.challenge_id=p.id)
	FROM public_challenges p JOIN users u ON u.id=p.user_id `

const challengeVisibility = `(p.user_id=$1 OR p.audience='public' OR
	(p.audience='friends' AND EXISTS (
		SELECT 1 FROM group_members author_members
		JOIN group_members viewer_members ON viewer_members.group_id=author_members.group_id
		WHERE author_members.user_id=p.user_id AND viewer_members.user_id=$1
	) AND (
		NOT EXISTS (SELECT 1 FROM public_challenge_groups selected WHERE selected.challenge_id=p.id)
		OR EXISTS (
			SELECT 1 FROM public_challenge_groups selected
			JOIN group_members selected_members ON selected_members.group_id=selected.group_id
			WHERE selected.challenge_id=p.id AND selected_members.user_id=$1
		)
	)))`

func scanPost(row interface{ Scan(...any) error }) (models.PublicChallenge, error) {
	var p models.PublicChallenge
	err := row.Scan(&p.ID, &p.UserID, &p.Username, &p.Avatar, &p.Caption, &p.CreatedAt, &p.Audience, &p.IsOwner, &p.Resolved, &p.ReactionCount, &p.Reacted, &p.CommentCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// authorizeProfileViewer applies the same visibility rule as the existing
// public profile endpoint before returning aggregate feed performance. A
// missing target and a target hidden by the existing shared-group rule remain
// distinguishable so the HTTP layer can preserve the profile API semantics.
func (r *Repository) authorizeProfileViewer(ctx context.Context, target, viewer string) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, target).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrProfileNotFound
	}
	if target == viewer {
		return nil
	}
	var shared bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM group_members target_members
		JOIN group_members viewer_members ON viewer_members.group_id=target_members.group_id
		WHERE target_members.user_id=$1 AND viewer_members.user_id=$2
	)`, target, viewer).Scan(&shared); err != nil {
		return err
	}
	if !shared {
		return ErrForbidden
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id, viewer string) (models.PublicChallenge, error) {
	return scanPost(r.pool.QueryRow(ctx, selectPost+`WHERE p.id=$2 AND `+challengeVisibility, viewer, id))
}

func (r *Repository) List(ctx context.Context, viewer string, cursor Cursor, limit int) (models.PublicFeedPage, error) {
	page := models.PublicFeedPage{Items: []models.PublicChallenge{}}
	query := selectPost + `WHERE ` + challengeVisibility + ` `
	args := []any{viewer, limit + 1}
	if cursor.ID != "" {
		// A direct seek predicate remains indexable with prepared generic plans.
		query += `AND (p.created_at,p.id)<($3,$4) `
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
	err := r.pool.QueryRow(ctx, `SELECT p.storage_key,p.mime_type,p.preview,
		p.user_id=$1 OR EXISTS (SELECT 1 FROM public_guesses WHERE challenge_id=$2 AND user_id=$1)
		FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility, viewer, id).Scan(&media.StorageKey, &media.MIMEType, &media.Preview, &media.Revealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return media, ErrNotFound
	}
	return media, err
}
