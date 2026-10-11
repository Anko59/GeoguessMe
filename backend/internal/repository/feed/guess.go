package feed

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/game"
	"geoguessme/internal/models"
	"geoguessme/internal/validation"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) Result(ctx context.Context, id, viewer string, now time.Time, hideDuration time.Duration) (models.PublicGuessResult, error) {
	var g models.PublicGuessResult
	var photo models.Photo
	err := r.pool.QueryRow(ctx, `SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long,p.user_id,p.hide_location,p.created_at
		FROM public_guesses g JOIN public_challenges p ON p.id=g.challenge_id
		WHERE g.challenge_id=$2 AND g.user_id=$1 AND `+challengeVisibility, viewer, id).
		Scan(&g.Score, &g.Distance, &g.Lat, &g.Long, &photo.Lat, &photo.Long, &photo.UserID, &photo.HideLocation, &photo.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if err == nil {
		g.ActualLat, g.ActualLong, g.LocationHidden, g.LocationRevealsAt = resultCoordinates(&photo, viewer, now, hideDuration)
	}
	return g, err
}

// Guess allows one immutable attempt. A shared parent lock excludes deletion
// without serializing different players. The unique key elects one winner;
// the following statement observes that winner after any conflict has committed.
func (r *Repository) Guess(ctx context.Context, id, viewer string, lat, long float64, now time.Time, hideDuration time.Duration) (models.PublicGuessResult, error) {
	var g models.PublicGuessResult
	if err := validation.ValidateCoordinates(lat, long); err != nil {
		return g, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var photo models.Photo
	err = tx.QueryRow(ctx, `SELECT p.user_id,p.lat,p.long,p.hide_location,p.created_at FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+` FOR KEY SHARE`, viewer, id).Scan(&photo.UserID, &photo.Lat, &photo.Long, &photo.HideLocation, &photo.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	if photo.UserID == viewer {
		return g, ErrForbidden
	}
	distance := game.CalculateDistance(lat, long, photo.Lat, photo.Long)
	_, err = tx.Exec(ctx, `INSERT INTO public_guesses(challenge_id,user_id,lat,long,score,distance)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (challenge_id,user_id) DO NOTHING`, id, viewer, lat, long, game.CalculateScore(distance), distance)
	if err != nil {
		return g, err
	}
	err = tx.QueryRow(ctx, `SELECT score,distance,lat,long FROM public_guesses WHERE challenge_id=$1 AND user_id=$2`, id, viewer).
		Scan(&g.Score, &g.Distance, &g.Lat, &g.Long)
	if err != nil {
		return g, err
	}
	g.ActualLat, g.ActualLong, g.LocationHidden, g.LocationRevealsAt = resultCoordinates(&photo, viewer, now, hideDuration)
	return g, tx.Commit(ctx)
}
