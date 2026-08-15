package groups

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/game"
	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// execer is satisfied by both the connection pool and a transaction, so the
// photo insert loop can run against either. It is the seam that lets the
// media-processing worker insert challenge photos and complete the job in one
// transaction.
type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// CreatePhotos inserts every photo atomically so a multi-group upload either
// lands in all selected groups or in none.
func (r *Repository) CreatePhotos(ctx context.Context, photos []*models.Photo) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.CreatePhotosTx(ctx, tx, photos); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreatePhotosTx inserts every photo using the supplied transaction. It is the
// tx-scoped variant used by the media-processing worker so the per-group
// challenge rows and the job completion commit atomically.
func (r *Repository) CreatePhotosTx(ctx context.Context, tx execer, photos []*models.Photo) error {
	for _, photo := range photos {
		if _, err := tx.Exec(ctx, `INSERT INTO photos (id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, hide_location, created_at, expires_at, retention_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`, photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt); err != nil {
			return err
		}
	}
	return nil
}

// Photo returns a photo by id, or nil when it does not exist.
func (r *Repository) Photo(ctx context.Context, id string) (*models.Photo, error) {
	return scanPhoto(r.pool.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, hide_location, created_at, expires_at, retention_at FROM photos WHERE id = $1`, id))
}

// ChallengeView is the accepted viewing state of a challenge.
type ChallengeView struct {
	PhotoID       string    `json:"photo_id"`
	UserID        string    `json:"user_id"`
	AcceptedAt    time.Time `json:"accepted_at"`
	ViewExpiresAt time.Time `json:"view_expires_at"`
}

// AcceptChallenge opens a private viewing window for a member on a challenge
// they did not post, in a single transaction. The membership and ownership
// rules are enforced here so the data layer and the transport layer share one
// access rule.
func (r *Repository) AcceptChallenge(ctx context.Context, photoID, userID string, viewWindow time.Duration, now time.Time) (*models.Photo, *ChallengeView, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	photo, err := scanPhoto(tx.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, hide_location, created_at, expires_at, retention_at FROM photos WHERE id = $1 FOR UPDATE`, photoID))
	if err != nil {
		return nil, nil, err
	}
	if photo == nil {
		return nil, nil, ErrNotFound
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, nil, err
	}
	if !member {
		return nil, nil, ErrForbidden
	}
	if photo.UserID == userID {
		return nil, nil, ErrOwnPhoto
	}
	if photo.ExpiresAt.Before(now) || photo.LifecycleStatus == "removed" {
		return nil, nil, ErrChallengeExpired
	}
	var view ChallengeView
	err = tx.QueryRow(ctx, `SELECT photo_id, user_id, accepted_at, view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&view.PhotoID, &view.UserID, &view.AcceptedAt, &view.ViewExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		view = ChallengeView{PhotoID: photoID, UserID: userID, AcceptedAt: now, ViewExpiresAt: now.Add(viewWindow)}
		if view.ViewExpiresAt.After(photo.ExpiresAt) {
			view.ViewExpiresAt = photo.ExpiresAt
		}
		if _, err := tx.Exec(ctx, `INSERT INTO challenge_views(photo_id, user_id, accepted_at, view_expires_at) VALUES ($1, $2, $3, $4)`, view.PhotoID, view.UserID, view.AcceptedAt, view.ViewExpiresAt); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return photo, &view, nil
}

// MarkMediaDelivered starts the one-time private viewing window. It is
// idempotent so the streaming handler and the client's delivery
// acknowledgement can safely race without extending an existing window.
func (r *Repository) MarkMediaDelivered(ctx context.Context, photoID, userID string, viewWindow time.Duration, now time.Time) (time.Time, error) {
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx, `
		UPDATE challenge_views v
		SET media_delivered_at = COALESCE(v.media_delivered_at, $3),
			view_expires_at = CASE
				WHEN v.media_delivered_at IS NULL THEN LEAST($3 + $4 * INTERVAL '1 second', p.expires_at)
				ELSE v.view_expires_at
			END
		FROM photos p
		WHERE v.photo_id = $1 AND v.user_id = $2 AND p.id = v.photo_id
		RETURNING v.view_expires_at`, photoID, userID, now, int64(viewWindow.Seconds())).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrForbidden
	}
	return expiresAt, err
}

// ViewDeliveryStatus reports the delivery state of a challenge view: whether
// the media was ever fully delivered and when the viewing window expires. A
// view row that does not exist is surfaced as pgx.ErrNoRows so callers keep
// the exact error handling they had when this query lived in the transport
// layer.
func (r *Repository) ViewDeliveryStatus(ctx context.Context, photoID, userID string) (delivered bool, viewExpiresAt time.Time, err error) {
	var deliveredAt pgtype.Timestamptz
	err = r.pool.QueryRow(ctx, `SELECT media_delivered_at, view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&deliveredAt, &viewExpiresAt)
	if err != nil {
		return false, time.Time{}, err
	}
	return deliveredAt.Valid, viewExpiresAt, nil
}

// GuessResult is the outcome of a guess submission.
type GuessResult struct {
	Guess    models.Guess
	Photo    *models.Photo
	Existing bool
}

// CanViewResults reports whether a member may view a challenge's results:
// owners and anyone after expiry may always view; others must have guessed.
func (r *Repository) CanViewResults(ctx context.Context, photoID, userID string, now time.Time) (*models.Photo, bool, error) {
	photo, err := r.Photo(ctx, photoID)
	if err != nil {
		return nil, false, err
	}
	if photo == nil {
		return nil, false, ErrNotFound
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, false, err
	}
	if !member {
		return nil, false, ErrForbidden
	}
	if photo.UserID == userID || !now.Before(photo.ExpiresAt) {
		return photo, true, nil
	}
	var guessed bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM guesses WHERE photo_id = $1 AND user_id = $2)`, photoID, userID).Scan(&guessed); err != nil {
		return nil, false, err
	}
	return photo, guessed, nil
}

// SubmitGuess records a guess for a challenge after validating the viewing
// window, retrying transient serialization failures within the documented
// limit. It is idempotent: a repeated guess returns the existing result.
func (r *Repository) SubmitGuess(ctx context.Context, photoID, userID string, lat, long float64, now time.Time) (*GuessResult, error) {
	if lat != lat || long != long || lat < -90 || lat > 90 || long < -180 || long > 180 {
		return nil, ErrInvalidCoordinate
	}
	const maxAttempts = 3
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, retry, err := r.submitGuessOnce(ctx, photoID, userID, lat, long, now)
		if !retry {
			return result, err
		}
		last = err
	}
	return nil, last
}

// submitGuessOnce performs a single guess attempt. It returns retry=true only
// for transient serialization/deadlock SQLSTATEs so the caller can try again
// within the documented limit.
func (r *Repository) submitGuessOnce(ctx context.Context, photoID, userID string, lat, long float64, now time.Time) (*GuessResult, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	photo, err := scanPhoto(tx.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, hide_location, created_at, expires_at, retention_at FROM photos WHERE id = $1 FOR UPDATE`, photoID))
	if err != nil {
		return nil, isRetryable(err), err
	}
	if photo == nil {
		return nil, false, ErrNotFound
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, isRetryable(err), err
	}
	if !member {
		return nil, false, ErrForbidden
	}
	if photo.UserID == userID {
		return nil, false, ErrOwnPhoto
	}
	var existing models.Guess
	err = tx.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.CreatedAt)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, isRetryable(err), err
		}
		return &GuessResult{Guess: existing, Photo: photo, Existing: true}, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, isRetryable(err), err
	}
	if photo.ExpiresAt.Before(now) {
		return nil, false, ErrChallengeExpired
	}
	var deliveredAt pgtype.Timestamptz
	var viewExpiresAt time.Time
	if err := tx.QueryRow(ctx, `SELECT media_delivered_at, view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&deliveredAt, &viewExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrForbidden
		}
		return nil, isRetryable(err), err
	}
	if !deliveredAt.Valid {
		return nil, false, ErrViewNotFinished
	}
	if now.Before(viewExpiresAt) {
		return nil, false, ErrViewNotFinished
	}
	distance := game.CalculateDistance(lat, long, photo.Lat, photo.Long)
	guess := models.Guess{ID: newID(), PhotoID: photoID, UserID: userID, GroupID: photo.GroupID, Lat: lat, Long: long, Score: game.CalculateScore(distance), Distance: distance, CreatedAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO guesses(id, photo_id, user_id, group_id, lat, long, score, distance, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, guess.Lat, guess.Long, guess.Score, guess.Distance, guess.CreatedAt); err != nil {
		// A concurrent duplicate lost the race; read the persisted winner.
		if isUniqueViolation(err) {
			return r.readExistingGuess(ctx, photoID, userID, photo)
		}
		return nil, isRetryable(err), err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, isRetryable(err), err
	}
	return &GuessResult{Guess: guess, Photo: photo}, false, nil
}

// readExistingGuess resolves the idempotent-duplicate case after a unique
// violation using a separate read so the original result is returned verbatim.
func (r *Repository) readExistingGuess(ctx context.Context, photoID, userID string, photo *models.Photo) (*GuessResult, bool, error) {
	var existing models.Guess
	err := r.pool.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	return &GuessResult{Guess: existing, Photo: photo, Existing: true}, false, nil
}

// GuessWithUser is a guess joined with its guesser's profile for results
// rendering.
type GuessWithUser struct {
	models.Guess
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// GuessesForPhoto returns every guess on a challenge with the guesser's
// profile, ordered by score descending then creation time ascending.
func (r *Repository) GuessesForPhoto(ctx context.Context, photoID string) ([]GuessWithUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id, g.photo_id, g.user_id, g.group_id, g.lat, g.long, g.score, g.distance, g.created_at, u.username, u.avatar FROM guesses g JOIN users u ON g.user_id = u.id WHERE g.photo_id = $1 ORDER BY g.score DESC, g.created_at ASC`, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var guesses []GuessWithUser
	for rows.Next() {
		var g GuessWithUser
		if err := rows.Scan(&g.ID, &g.PhotoID, &g.UserID, &g.GroupID, &g.Lat, &g.Long, &g.Score, &g.Distance, &g.CreatedAt, &g.Username, &g.Avatar); err != nil {
			return nil, err
		}
		guesses = append(guesses, g)
	}
	return guesses, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isRetryable reports serialization/deadlock SQLSTATEs documented as safe to
// retry. All other errors are surfaced immediately.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01": // serialization_failure, deadlock_detected
		return true
	}
	return false
}

func newID() string {
	return uuid.NewString()
}
