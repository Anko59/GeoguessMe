package feed

import (
	"context"
	"errors"

	"geoguessme/internal/game"
	"geoguessme/internal/models"
	"geoguessme/internal/validation"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) Result(ctx context.Context, id, viewer string) (models.PublicGuessResult, error) {
	var g models.PublicGuessResult
	err := r.pool.QueryRow(ctx, `SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long
		FROM public_guesses g JOIN public_challenges p ON p.id=g.challenge_id
		WHERE g.challenge_id=$1 AND g.user_id=$2`, id, viewer).
		Scan(&g.Score, &g.Distance, &g.Lat, &g.Long, &g.ActualLat, &g.ActualLong)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

// Guess allows one immutable attempt. A shared parent lock excludes deletion
// without serializing different players. The unique key elects one winner;
// the following statement observes that winner after any conflict has committed.
func (r *Repository) Guess(ctx context.Context, id, viewer string, lat, long float64) (models.PublicGuessResult, error) {
	var g models.PublicGuessResult
	if err := validation.ValidateCoordinates(lat, long); err != nil {
		return g, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner string
	err = tx.QueryRow(ctx, `SELECT user_id,lat,long FROM public_challenges WHERE id=$1 FOR KEY SHARE`, id).Scan(&owner, &g.ActualLat, &g.ActualLong)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	if owner == viewer {
		return g, ErrForbidden
	}
	distance := game.CalculateDistance(lat, long, g.ActualLat, g.ActualLong)
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
	return g, tx.Commit(ctx)
}
