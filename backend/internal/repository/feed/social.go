package feed

import (
	"context"
	"errors"

	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) React(ctx context.Context, id, viewer string, liked bool) error {
	if !liked {
		_, err := r.pool.Exec(ctx, `DELETE FROM public_reactions WHERE challenge_id=$1 AND user_id=$2`, id, viewer)
		return err
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `WITH challenge AS (SELECT id FROM public_challenges WHERE id=$1),
		inserted AS (INSERT INTO public_reactions(challenge_id,user_id)
		SELECT id,$2 FROM challenge WHERE true ON CONFLICT (challenge_id,user_id) DO NOTHING)
		SELECT EXISTS (SELECT 1 FROM challenge)`, id, viewer).Scan(&exists)
	if err == nil && !exists {
		return ErrNotFound
	}
	return err
}

func (r *Repository) Comments(ctx context.Context, id, viewer string, cursor Cursor, limit int) (models.PublicCommentsPage, error) {
	page := models.PublicCommentsPage{Items: []models.PublicComment{}}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public_challenges WHERE id=$1)`, id).Scan(&exists); err != nil {
		return page, err
	}
	if !exists {
		return page, ErrNotFound
	}
	query := `SELECT c.id,c.user_id,u.username,c.content,c.created_at,
		c.user_id=$2 OR p.user_id=$2 FROM public_comments c
		JOIN users u ON u.id=c.user_id JOIN public_challenges p ON p.id=c.challenge_id
		WHERE c.challenge_id=$1 `
	args := []any{id, viewer, limit + 1}
	if cursor.ID != "" {
		query += `AND (c.created_at,c.id)<($4,$5) `
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	rows, err := r.pool.Query(ctx, query+`ORDER BY c.created_at DESC,c.id DESC LIMIT $3`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var c models.PublicComment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Username, &c.Content, &c.CreatedAt, &c.CanDelete); err != nil {
			return page, err
		}
		page.Items = append(page.Items, c)
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

func (r *Repository) Comment(ctx context.Context, id, viewer, content string) (models.PublicComment, error) {
	c := models.PublicComment{ID: uuid.NewString(), UserID: viewer, Content: content, CanDelete: true}
	err := r.pool.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO public_comments(id,challenge_id,user_id,content)
		SELECT $1,id,$3,$4 FROM public_challenges WHERE id=$2 RETURNING created_at)
		SELECT inserted.created_at,u.username FROM inserted JOIN users u ON u.id=$3`, c.ID, id, viewer, content).
		Scan(&c.CreatedAt, &c.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Authors can remove their comments; post owners can moderate their threads.
func (r *Repository) DeleteComment(ctx context.Context, id, commentID, viewer string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM public_comments c USING public_challenges p
		WHERE c.challenge_id=p.id AND p.id=$1 AND c.id=$2 AND (c.user_id=$3 OR p.user_id=$3)`, id, commentID, viewer)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
