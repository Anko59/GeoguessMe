package feed

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/game"
	"geoguessme/internal/models"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type PublicChallengeView struct {
	AcceptedAt     time.Time
	ViewExpiresAt  time.Time
	GuessExpiresAt time.Time
	Delivered      bool
	MediaType      string
}

// AcceptTimedChallenge creates the viewer's server-owned timed session. The
// insert is idempotent so reopening a challenge never resets its deadlines.
func (r *Repository) AcceptTimedChallenge(ctx context.Context, id, viewer string, viewWindow, guessWindow time.Duration, now time.Time) (PublicChallengeView, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PublicChallengeView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner, mediaType string
	if err := tx.QueryRow(ctx, `SELECT p.user_id,p.mime_type FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+` FOR UPDATE`, viewer, id).Scan(&owner, &mediaType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicChallengeView{}, ErrNotFound
		}
		return PublicChallengeView{}, err
	}
	if owner == viewer {
		return PublicChallengeView{}, ErrOwnChallenge
	}
	_, err = tx.Exec(ctx, `INSERT INTO public_challenge_views(challenge_id,user_id,accepted_at,view_expires_at,guess_expires_at)
		VALUES ($1,$2,$3,$3+$4 * INTERVAL '1 second',$3+$4 * INTERVAL '1 second'+$5 * INTERVAL '1 second')
		ON CONFLICT (challenge_id,user_id) DO NOTHING`, id, viewer, now, intervalSeconds(viewWindow), intervalSeconds(guessWindow))
	if err != nil {
		return PublicChallengeView{}, err
	}
	view, err := scanPublicView(tx.QueryRow(ctx, `SELECT accepted_at,media_delivered_at,view_expires_at,guess_expires_at
		FROM public_challenge_views WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err != nil {
		return PublicChallengeView{}, err
	}
	view.MediaType = mediaType
	return view, tx.Commit(ctx)
}

func intervalSeconds(d time.Duration) int64 { return int64(d / time.Second) }

func scanPublicView(row interface{ Scan(...any) error }) (PublicChallengeView, error) {
	var view PublicChallengeView
	var deliveredAt pgtype.Timestamptz
	err := row.Scan(&view.AcceptedAt, &deliveredAt, &view.ViewExpiresAt, &view.GuessExpiresAt)
	view.Delivered = deliveredAt.Valid
	return view, err
}

// MarkTimedMediaDelivered starts the authoritative window only once the
// complete original image has reached the client. Repeated acknowledgements
// return the original deadlines and cannot extend play time.
func (r *Repository) MarkTimedMediaDelivered(ctx context.Context, id, viewer string, viewWindow, guessWindow time.Duration, now time.Time) (PublicChallengeView, error) {
	var view PublicChallengeView
	var deliveredAt pgtype.Timestamptz
	err := r.pool.QueryRow(ctx, `UPDATE public_challenge_views v SET
		media_delivered_at=COALESCE(v.media_delivered_at,$3),
		view_expires_at=CASE WHEN v.media_delivered_at IS NULL THEN $3+$4 * INTERVAL '1 second' ELSE v.view_expires_at END,
		guess_expires_at=CASE WHEN v.media_delivered_at IS NULL THEN $3+$4 * INTERVAL '1 second'+$5 * INTERVAL '1 second' ELSE v.guess_expires_at END
		FROM public_challenges p
		WHERE v.challenge_id=$1 AND v.user_id=$2 AND p.id=v.challenge_id AND `+challengeVisibility+`
		RETURNING v.media_delivered_at,v.view_expires_at,v.guess_expires_at`, id, viewer, now, intervalSeconds(viewWindow), intervalSeconds(guessWindow)).Scan(&deliveredAt, &view.ViewExpiresAt, &view.GuessExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicChallengeView{}, ErrForbidden
	}
	view.Delivered = deliveredAt.Valid
	return view, err
}

func (r *Repository) TimedMedia(ctx context.Context, id, viewer string, now time.Time) (Media, error) {
	var media Media
	err := r.pool.QueryRow(ctx, `SELECT p.storage_key,p.mime_type,p.preview
		FROM public_challenges p
		LEFT JOIN public_challenge_views v ON v.challenge_id=p.id AND v.user_id=$1
		WHERE p.id=$2 AND `+challengeVisibility+` AND
		(p.user_id=$1 OR (v.challenge_id IS NOT NULL AND (v.media_delivered_at IS NULL OR $3 < v.view_expires_at OR EXISTS (SELECT 1 FROM public_guesses g WHERE g.challenge_id=p.id AND g.user_id=$1))))`, viewer, id, now).Scan(&media.StorageKey, &media.MIMEType, &media.Preview)
	if errors.Is(err, pgx.ErrNoRows) {
		// Keep deadline failures distinguishable from visibility failures for the
		// timed UI, while preserving a not-found response for invisible posts.
		var visible bool
		if visibilityErr := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+`)`, viewer, id).Scan(&visible); visibilityErr != nil {
			return media, visibilityErr
		}
		if visible {
			return media, ErrMediaExpired
		}
		return media, ErrNotFound
	}
	return media, err
}

type TimedGuessResult struct {
	ID          string
	ChallengeID string
	UserID      string
	Lat         float64
	Long        float64
	Score       int
	Distance    float64
	TimedOut    bool
	CreatedAt   time.Time
}

func scanTimedGuess(row interface{ Scan(...any) error }) (TimedGuessResult, error) {
	var guess TimedGuessResult
	err := row.Scan(&guess.ID, &guess.ChallengeID, &guess.UserID, &guess.Lat, &guess.Long, &guess.Score, &guess.Distance, &guess.TimedOut, &guess.CreatedAt)
	return guess, err
}

func (r *Repository) TimedGuess(ctx context.Context, id, viewer string, lat, long float64, now time.Time) (TimedGuessResult, bool, error) {
	if err := validation.ValidateCoordinates(lat, long); err != nil {
		return TimedGuessResult{}, false, ErrInvalidCoordinate
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TimedGuessResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner string
	var actualLat, actualLong float64
	if err := tx.QueryRow(ctx, `SELECT p.user_id,p.lat,p.long FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+` FOR UPDATE`, viewer, id).Scan(&owner, &actualLat, &actualLong); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimedGuessResult{}, false, ErrNotFound
		}
		return TimedGuessResult{}, false, err
	}
	if owner == viewer {
		return TimedGuessResult{}, false, ErrOwnChallenge
	}
	existing, err := scanTimedGuess(tx.QueryRow(ctx, `SELECT id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at FROM public_guesses WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return TimedGuessResult{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TimedGuessResult{}, false, err
	}
	view, err := scanPublicView(tx.QueryRow(ctx, `SELECT accepted_at,media_delivered_at,view_expires_at,guess_expires_at FROM public_challenge_views WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimedGuessResult{}, false, ErrForbidden
		}
		return TimedGuessResult{}, false, err
	}
	if !view.Delivered || now.Before(view.ViewExpiresAt) {
		return TimedGuessResult{}, false, ErrViewNotFinished
	}
	if !now.Before(view.GuessExpiresAt) {
		if err := ensureTimedTimeout(ctx, tx, id, viewer, now); err != nil {
			return TimedGuessResult{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return TimedGuessResult{}, false, err
		}
		return TimedGuessResult{}, false, ErrGuessTimeExpired
	}
	distance := game.CalculateDistance(lat, long, actualLat, actualLong)
	elapsed := now.Sub(view.ViewExpiresAt)
	if elapsed < 0 {
		elapsed = 0
	}
	guess := TimedGuessResult{ID: uuid.NewString(), ChallengeID: id, UserID: viewer, Lat: lat, Long: long, Distance: distance, Score: game.CalculateScoreWithTime(distance, elapsed, view.GuessExpiresAt.Sub(view.ViewExpiresAt)), CreatedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO public_guesses(id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,FALSE,$8) ON CONFLICT (challenge_id,user_id) DO NOTHING`, guess.ID, guess.ChallengeID, guess.UserID, guess.Lat, guess.Long, guess.Score, guess.Distance, guess.CreatedAt)
	if err != nil {
		return TimedGuessResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TimedGuessResult{}, false, err
	}
	return guess, false, nil
}

func ensureTimedTimeout(ctx context.Context, tx pgx.Tx, id, viewer string, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public_guesses(id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at)
		VALUES ($1,$2,$3,0,0,0,0,TRUE,$4) ON CONFLICT (challenge_id,user_id) DO NOTHING`, uuid.NewString(), id, viewer, now)
	return err
}

func (r *Repository) TimedTimeout(ctx context.Context, id, viewer string, now time.Time) (TimedGuessResult, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TimedGuessResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner string
	if err := tx.QueryRow(ctx, `SELECT p.user_id FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+` FOR UPDATE`, viewer, id).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimedGuessResult{}, false, ErrNotFound
		}
		return TimedGuessResult{}, false, err
	}
	if owner == viewer {
		return TimedGuessResult{}, false, ErrOwnChallenge
	}
	existing, err := scanTimedGuess(tx.QueryRow(ctx, `SELECT id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at FROM public_guesses WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return TimedGuessResult{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TimedGuessResult{}, false, err
	}
	view, err := scanPublicView(tx.QueryRow(ctx, `SELECT accepted_at,media_delivered_at,view_expires_at,guess_expires_at FROM public_challenge_views WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimedGuessResult{}, false, ErrForbidden
		}
		return TimedGuessResult{}, false, err
	}
	if !view.Delivered || now.Before(view.ViewExpiresAt) || now.Before(view.GuessExpiresAt) {
		return TimedGuessResult{}, false, ErrViewNotFinished
	}
	if err := ensureTimedTimeout(ctx, tx, id, viewer, now); err != nil {
		return TimedGuessResult{}, false, err
	}
	result, err := scanTimedGuess(tx.QueryRow(ctx, `SELECT id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at FROM public_guesses WHERE challenge_id=$1 AND user_id=$2`, id, viewer))
	if err != nil {
		return TimedGuessResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TimedGuessResult{}, false, err
	}
	return result, false, nil
}

func (r *Repository) TimedResults(ctx context.Context, id, viewer string, now time.Time) (models.PublicTimedResults, error) {
	var result models.PublicTimedResults
	result.ChallengeID = id
	var owner string
	if err := r.pool.QueryRow(ctx, `SELECT p.user_id,p.lat,p.long FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility, viewer, id).Scan(&owner, &result.ActualLat, &result.ActualLong); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return result, ErrNotFound
		}
		return result, err
	}
	if owner != viewer {
		var allowed bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public_guesses WHERE challenge_id=$1 AND user_id=$2)
			OR EXISTS (SELECT 1 FROM public_challenge_views WHERE challenge_id=$1 AND user_id=$2 AND media_delivered_at IS NOT NULL AND guess_expires_at <= $3)`, id, viewer, now).Scan(&allowed); err != nil {
			return result, err
		}
		if !allowed {
			return result, ErrForbidden
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT g.id,g.user_id,u.username,u.avatar,g.lat,g.long,g.score,g.distance,g.timed_out,g.created_at
		FROM public_guesses g JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		WHERE g.challenge_id=$1 ORDER BY g.score DESC,g.created_at ASC,g.user_id ASC`, id)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Guesses = []models.PublicTimedResultGuess{}
	for rows.Next() {
		var guess models.PublicTimedResultGuess
		var lat, long, distance float64
		if err := rows.Scan(&guess.ID, &guess.UserID, &guess.Username, &guess.Avatar, &lat, &long, &guess.Score, &distance, &guess.TimedOut, &guess.CreatedAt); err != nil {
			return result, err
		}
		if !guess.TimedOut {
			guess.Lat, guess.Long, guess.Distance = &lat, &long, &distance
		}
		result.Guesses = append(result.Guesses, guess)
	}
	return result, rows.Err()
}
