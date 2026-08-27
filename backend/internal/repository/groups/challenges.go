package groups

import (
	"context"
	"errors"
	"fmt"
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
	PhotoID        string    `json:"photo_id"`
	UserID         string    `json:"user_id"`
	AcceptedAt     time.Time `json:"accepted_at"`
	ViewExpiresAt  time.Time `json:"view_expires_at"`
	GuessExpiresAt time.Time `json:"guess_expires_at"`
}

// AcceptChallenge opens a private viewing window for a member on a challenge
// they did not post, in a single transaction. The membership and ownership
// rules are enforced here so the data layer and the transport layer share one
// access rule. The guess window (view end + guessWindow, capped at the
// challenge expiry) is recorded alongside the view so the guess deadline is
// fixed at acceptance time even when the player closes the app.
func (r *Repository) AcceptChallenge(ctx context.Context, photoID, userID string, viewWindow, guessWindow time.Duration, now time.Time) (*models.Photo, *ChallengeView, error) {
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
	var guessExpires pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT photo_id, user_id, accepted_at, view_expires_at, guess_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&view.PhotoID, &view.UserID, &view.AcceptedAt, &view.ViewExpiresAt, &guessExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		view = ChallengeView{PhotoID: photoID, UserID: userID, AcceptedAt: now, ViewExpiresAt: now.Add(viewWindow)}
		if view.ViewExpiresAt.After(photo.ExpiresAt) {
			view.ViewExpiresAt = photo.ExpiresAt
		}
		// The guess window opens when the viewing window ends; it is capped at
		// the challenge expiry so a guess deadline can never outlive the photo.
		view.GuessExpiresAt = view.ViewExpiresAt.Add(guessWindow)
		if view.GuessExpiresAt.After(photo.ExpiresAt) {
			view.GuessExpiresAt = photo.ExpiresAt
		}
		if _, err := tx.Exec(ctx, `INSERT INTO challenge_views(photo_id, user_id, accepted_at, view_expires_at, guess_expires_at) VALUES ($1, $2, $3, $4, $5)`, view.PhotoID, view.UserID, view.AcceptedAt, view.ViewExpiresAt, view.GuessExpiresAt); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else if !guessExpires.Valid {
		// A legacy view row without a recorded guess deadline (pre-migration)
		// derives it from the stored view end so the accept response stays
		// valid even before migration 021 backfills the row.
		view.GuessExpiresAt = view.ViewExpiresAt.Add(guessWindow)
		if view.GuessExpiresAt.After(photo.ExpiresAt) {
			view.GuessExpiresAt = photo.ExpiresAt
		}
	} else {
		view.GuessExpiresAt = guessExpires.Time
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return photo, &view, nil
}

// MarkMediaDelivered starts the one-time private viewing window and the
// derived guess window. It is idempotent so the streaming handler and the
// client's delivery acknowledgement can safely race without extending an
// existing window; on a first delivery both deadlines move to delivery + the
// respective windows (capped at the challenge expiry) and are never extended
// again.
func (r *Repository) MarkMediaDelivered(ctx context.Context, photoID, userID string, viewWindow, guessWindow time.Duration, now time.Time) (viewExpiresAt, guessExpiresAt time.Time, err error) {
	err = r.pool.QueryRow(ctx, `
		UPDATE challenge_views v
		SET media_delivered_at = COALESCE(v.media_delivered_at, $3),
			view_expires_at = CASE
				WHEN v.media_delivered_at IS NULL THEN LEAST($3 + $4 * INTERVAL '1 second', p.expires_at)
				ELSE v.view_expires_at
			END,
			guess_expires_at = CASE
				WHEN v.media_delivered_at IS NULL THEN LEAST($3 + $4 * INTERVAL '1 second' + $5 * INTERVAL '1 second', p.expires_at)
				ELSE v.guess_expires_at
			END
		FROM photos p
		WHERE v.photo_id = $1 AND v.user_id = $2 AND p.id = v.photo_id
		RETURNING v.view_expires_at, v.guess_expires_at`, photoID, userID, now, int64(viewWindow.Seconds()), int64(guessWindow.Seconds())).Scan(&viewExpiresAt, &guessExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, time.Time{}, ErrForbidden
	}
	return viewExpiresAt, guessExpiresAt, err
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
	err = tx.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.TimedOut, &existing.CreatedAt)
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
	var guessExpiresAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `SELECT media_delivered_at, view_expires_at, guess_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&deliveredAt, &viewExpiresAt, &guessExpiresAt); err != nil {
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
	// The guess window is server-authoritative: a guess submitted after the
	// recorded deadline is refused even when the client lost its timer (for
	// example because the app was closed), so "did not guess in time" cannot
	// be bypassed by reopening the challenge. Legacy rows without a recorded
	// deadline (NULL) keep the pre-window behavior. When the deadline has
	// passed, the server records a timed-out guess (score 0) so the player
	// still appears in results.
	if guessExpiresAt.Valid && !now.Before(guessExpiresAt.Time) {
		// Recording the timed-out row is part of the guarantee that a player
		// who misses the window still appears in results, so an insert or
		// commit failure must surface (500 + observability) instead of
		// silently degrading to a 410 without a persisted timeout row.
		if _, err := r.ensureTimeoutGuess(ctx, tx, photoID, userID, photo.GroupID, now); err != nil {
			_ = tx.Rollback(ctx) // best effort: the transaction is being aborted regardless
			return nil, isRetryable(err), fmt.Errorf("record timed-out guess: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, isRetryable(err), err
		}
		return nil, false, ErrGuessTimeExpired
	}
	distance := game.CalculateDistance(lat, long, photo.Lat, photo.Long)
	elapsed := now.Sub(viewExpiresAt)
	if elapsed < 0 {
		elapsed = 0
	}
	var guessWindow time.Duration
	if guessExpiresAt.Valid {
		guessWindow = guessExpiresAt.Time.Sub(viewExpiresAt)
	} else {
		guessWindow = 5 * time.Minute
	}
	score := game.CalculateScoreWithTime(distance, elapsed, guessWindow)
	guess := models.Guess{ID: newID(), PhotoID: photoID, UserID: userID, GroupID: photo.GroupID, Lat: lat, Long: long, Score: score, Distance: distance, CreatedAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO guesses(id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, guess.Lat, guess.Long, guess.Score, guess.Distance, false, guess.CreatedAt); err != nil {
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

// ensureTimeoutGuess records a timed-out guess (score 0) when the guess window
// expired without a guess, so the player appears in results with "timed out".
func (r *Repository) ensureTimeoutGuess(ctx context.Context, tx pgx.Tx, photoID, userID, groupID string, now time.Time) (*models.Guess, error) {
	guess := models.Guess{ID: newID(), PhotoID: photoID, UserID: userID, GroupID: groupID, Score: 0, TimedOut: true, CreatedAt: now}
	_, err := tx.Exec(ctx, `INSERT INTO guesses(id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (photo_id, user_id) DO NOTHING`, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, 0, 0, 0, 0, true, guess.CreatedAt)
	return &guess, err
}

// TimeoutGuess records a timed-out guess for the caller without coordinates.
// It is idempotent and used when the client's countdown expires.
func (r *Repository) TimeoutGuess(ctx context.Context, photoID, userID string, now time.Time) (*GuessResult, error) {
	photo, err := r.Photo(ctx, photoID)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, ErrNotFound
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, err
	}
	if !member {
		return nil, ErrForbidden
	}
	if photo.UserID == userID {
		return nil, ErrOwnPhoto
	}
	var existing models.Guess
	err = r.pool.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.TimedOut, &existing.CreatedAt)
	if err == nil {
		return &GuessResult{Guess: existing, Photo: photo, Existing: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var deliveredAt pgtype.Timestamptz
	var viewExpiresAt time.Time
	var guessExpiresAt pgtype.Timestamptz
	if err := r.pool.QueryRow(ctx, `SELECT media_delivered_at, view_expires_at, guess_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&deliveredAt, &viewExpiresAt, &guessExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	if !deliveredAt.Valid || now.Before(viewExpiresAt) {
		return nil, ErrViewNotFinished
	}
	// The client fires POST /timeout when its local countdown hits zero, which
	// can be a tick or two before the authoritative server deadline (clock and
	// render-tick skew). Accept such calls inside a small tolerance so the
	// guaranteed timeout row is not lost to the race; anything earlier is
	// still refused as viewing_window_open.
	const timeoutSkew = 2 * time.Second
	if guessExpiresAt.Valid && now.Before(guessExpiresAt.Time.Add(-timeoutSkew)) {
		return nil, ErrViewNotFinished
	}
	if photo.ExpiresAt.Before(now) {
		return nil, ErrChallengeExpired
	}
	guess := models.Guess{ID: newID(), PhotoID: photoID, UserID: userID, GroupID: photo.GroupID, Score: 0, TimedOut: true, CreatedAt: now}
	_, err = r.pool.Exec(ctx, `INSERT INTO guesses(id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (photo_id, user_id) DO NOTHING`, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, 0, 0, 0, 0, true, guess.CreatedAt)
	if err != nil {
		return nil, err
	}
	var inserted models.Guess
	err = r.pool.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&inserted.ID, &inserted.PhotoID, &inserted.UserID, &inserted.GroupID, &inserted.Lat, &inserted.Long, &inserted.Score, &inserted.Distance, &inserted.TimedOut, &inserted.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &GuessResult{Guess: inserted, Photo: photo}, nil
}

// readExistingGuess resolves the idempotent-duplicate case after a unique
// violation using a separate read so the original result is returned verbatim.
func (r *Repository) readExistingGuess(ctx context.Context, photoID, userID string, photo *models.Photo) (*GuessResult, bool, error) {
	var existing models.Guess
	err := r.pool.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, timed_out, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.TimedOut, &existing.CreatedAt)
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
	rows, err := r.pool.Query(ctx, `SELECT g.id, g.photo_id, g.user_id, g.group_id, g.lat, g.long, g.score, g.distance, g.timed_out, g.created_at, u.username, u.avatar FROM guesses g JOIN users u ON g.user_id = u.id WHERE g.photo_id = $1 ORDER BY g.score DESC, g.created_at ASC`, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var guesses []GuessWithUser
	for rows.Next() {
		var g GuessWithUser
		if err := rows.Scan(&g.ID, &g.PhotoID, &g.UserID, &g.GroupID, &g.Lat, &g.Long, &g.Score, &g.Distance, &g.TimedOut, &g.CreatedAt, &g.Username, &g.Avatar); err != nil {
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
