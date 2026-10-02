package pins

import (
	"context"
	"errors"
	"fmt"

	"geoguessme/internal/geography"
)

func (r *Repository) scoreCount(ctx context.Context, userID string, criteria challengeCriteria, allowLegacyPartyScore bool) (bool, error) {
	if criteria.Count <= 0 || (criteria.Score <= 0 && criteria.MinimumScore <= 0) || (criteria.Score > 0 && criteria.MinimumScore > 0) {
		return false, errors.New("score count requires a positive count and exactly one score threshold")
	}
	source := criteria.Source
	if source == "" {
		source = "all"
	}
	var query string
	switch source {
	case "all":
		query = `SELECT score, 'group' AS score_source FROM guesses WHERE user_id = $1 AND NOT timed_out UNION ALL SELECT score, 'public' AS score_source FROM public_guesses WHERE user_id = $1 AND NOT timed_out`
	case "group":
		query = `SELECT score, 'group' AS score_source FROM guesses WHERE user_id = $1 AND NOT timed_out`
	case "public":
		query = `SELECT score, 'public' AS score_source FROM public_guesses WHERE user_id = $1 AND NOT timed_out`
	default:
		return false, fmt.Errorf("score count has unsupported source %q", source)
	}
	var condition string
	if criteria.Score > 0 {
		condition = "score = $2"
		if allowLegacyPartyScore && criteria.Score == 5000 && source != "public" {
			condition = "score = $2 OR (score = 10000 AND score_source = 'group')"
		}
	} else {
		condition = "score >= $2"
	}
	var eligible bool
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) >= $3 FROM (`+query+`) scored WHERE `+condition, userID, scoreThreshold(criteria), criteria.Count).Scan(&eligible)
	return eligible, err
}

func scoreThreshold(criteria challengeCriteria) int {
	if criteria.Score > 0 {
		return criteria.Score
	}
	return criteria.MinimumScore
}

func (r *Repository) hasScoreStreak(ctx context.Context, userID string, criteria challengeCriteria) (bool, error) {
	if criteria.Count <= 0 || criteria.MinimumScore <= 0 {
		return false, errors.New("score_streak requires a positive score and count")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT score, timed_out FROM (
			SELECT score, timed_out, created_at, photo_id AS challenge_id, 0 AS source_order FROM guesses WHERE user_id = $1
			UNION ALL
			SELECT score, timed_out, created_at, challenge_id, 1 AS source_order FROM public_guesses WHERE user_id = $1
		) attempts
		ORDER BY created_at, challenge_id, source_order`, userID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	streak := 0
	for rows.Next() {
		var score int
		var timedOut bool
		if err := rows.Scan(&score, &timedOut); err != nil {
			return false, err
		}
		if timedOut || score < criteria.MinimumScore {
			streak = 0
			continue
		}
		streak++
		if streak >= criteria.Count {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *Repository) distanceCount(ctx context.Context, userID string, criteria challengeCriteria) (bool, error) {
	if criteria.Count <= 0 || criteria.MaximumDistanceMeters <= 0 && criteria.MinimumDistanceMeters <= 0 {
		return false, errors.New("distance count requires a positive count and distance bound")
	}
	if criteria.MaximumDistanceMeters > 0 && criteria.MinimumDistanceMeters > 0 {
		return false, errors.New("distance count cannot use both minimum and maximum distance bounds")
	}
	var condition string
	var bound float64
	if criteria.MaximumDistanceMeters > 0 {
		condition = "distance <= $2"
		bound = criteria.MaximumDistanceMeters
	} else {
		condition = "distance >= $2"
		bound = criteria.MinimumDistanceMeters
	}
	var scoreCondition string
	countPosition := 3
	if criteria.MinimumScore > 0 {
		scoreCondition = " AND score >= $3"
		countPosition = 4
	}
	query := fmt.Sprintf(`SELECT COUNT(*) >= $%d FROM (
		SELECT distance, score FROM guesses WHERE user_id = $1 AND NOT timed_out
		UNION ALL
		SELECT distance, score FROM public_guesses WHERE user_id = $1 AND NOT timed_out
	) scored WHERE `+condition+scoreCondition, countPosition)
	var eligible bool
	if criteria.MinimumScore > 0 {
		err := r.pool.QueryRow(ctx, query, userID, bound, criteria.MinimumScore, criteria.Count).Scan(&eligible)
		return eligible, err
	}
	err := r.pool.QueryRow(ctx, query, userID, bound, criteria.Count).Scan(&eligible)
	return eligible, err
}

func (r *Repository) timedPublicCount(ctx context.Context, userID string, criteria challengeCriteria) (bool, error) {
	if criteria.Count <= 0 {
		return false, errors.New("timed_public_count requires a positive count")
	}
	var eligible bool
	if criteria.Seconds > 0 {
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) >= $3
			FROM public_guesses g
			JOIN public_challenge_views v ON v.challenge_id = g.challenge_id AND v.user_id = g.user_id
			WHERE g.user_id = $1 AND NOT g.timed_out
			  AND g.created_at >= v.view_expires_at
			  AND g.created_at <= v.guess_expires_at
			  AND g.created_at <= v.view_expires_at + $2 * INTERVAL '1 second'`, userID, criteria.Seconds, criteria.Count).Scan(&eligible)
		return eligible, err
	}
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) >= $2
		FROM public_guesses g
		JOIN public_challenge_views v ON v.challenge_id = g.challenge_id AND v.user_id = g.user_id
		WHERE g.user_id = $1 AND NOT g.timed_out
		  AND g.created_at >= v.view_expires_at
		  AND g.created_at <= v.guess_expires_at`, userID, criteria.Count).Scan(&eligible)
	return eligible, err
}

func (r *Repository) sourceScoreCount(ctx context.Context, userID string, criteria challengeCriteria) (bool, error) {
	if criteria.GroupCount <= 0 || criteria.PublicCount <= 0 || criteria.MinimumScore <= 0 {
		return false, errors.New("source_score_count requires group/public counts and a positive score")
	}
	var groupCount, publicCount int
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE source = 'group'),
			COUNT(*) FILTER (WHERE source = 'public')
		FROM (
			SELECT 'group' AS source, score FROM guesses WHERE user_id = $1 AND NOT timed_out
			UNION ALL
			SELECT 'public' AS source, score FROM public_guesses WHERE user_id = $1 AND NOT timed_out
		) scored
		WHERE score >= $2`, userID, criteria.MinimumScore).Scan(&groupCount, &publicCount)
	return groupCount >= criteria.GroupCount && publicCount >= criteria.PublicCount, err
}

func (r *Repository) hasScoresInEnoughCountries(ctx context.Context, userID string, minimumScore, required int) (bool, error) {
	index, err := r.countryBoundaries()
	if err != nil {
		return false, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT lat, long FROM (
			SELECT DISTINCT p.lat, p.long
			FROM guesses g JOIN photos p ON p.id = g.photo_id
			WHERE g.user_id = $1 AND NOT g.timed_out AND g.score >= $2
			UNION
			SELECT DISTINCT p.lat, p.long
			FROM public_guesses g JOIN public_challenges p ON p.id = g.challenge_id
			WHERE g.user_id = $1 AND NOT g.timed_out AND g.score >= $2
		) scored_locations`, userID, minimumScore)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	countries := make(map[string]struct{}, required)
	for rows.Next() {
		var lat, lon float64
		if err := rows.Scan(&lat, &lon); err != nil {
			return false, err
		}
		if code, found := index.CountryAt(lat, lon); found {
			countries[code] = struct{}{}
			if len(countries) >= required {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

func (r *Repository) countryBoundaries() (*geography.Index, error) {
	r.countryOnce.Do(func() {
		r.countries, r.countryErr = geography.LoadNaturalEarthIndex()
	})
	if r.countryErr != nil {
		return nil, r.countryErr
	}
	return r.countries, nil
}
