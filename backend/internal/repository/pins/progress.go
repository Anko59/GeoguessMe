package pins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type challengeCriteria struct {
	Kind                  string  `json:"kind"`
	Score                 int     `json:"score"`
	Count                 int     `json:"count"`
	Rank                  int     `json:"rank"`
	TiePolicy             string  `json:"tie_policy"`
	WeekStart             string  `json:"week_start"`
	ThresholdExclusive    int     `json:"threshold_exclusive"`
	MinimumScore          int     `json:"minimum_score"`
	MinimumDistanceMeters float64 `json:"minimum_distance_meters"`
	MaximumDistanceMeters float64 `json:"maximum_distance_meters"`
	Countries             int     `json:"countries"`
	Source                string  `json:"source"`
	StartUTCHour          int     `json:"start_utc_hour"`
	EndUTCHour            int     `json:"end_utc_hour"`
	Seconds               int     `json:"seconds"`
	GroupCount            int     `json:"group_count"`
	PublicCount           int     `json:"public_count"`
}

type unlockChallenge struct {
	key      string
	criteria challengeCriteria
}

// unlockEligibleChallenges persists every challenge the player has already
// completed. This action lets existing accounts earn credit for challenge
// results recorded before the catalog was seeded.
func (r *Repository) unlockEligibleChallenges(ctx context.Context, userID string, now time.Time) error {
	rows, err := r.pool.Query(ctx, `
		SELECT c.challenge_key, c.criteria
		FROM map_pin_challenges c
		JOIN map_pins p ON p.pin_key = c.pin_key AND p.is_active
		WHERE c.is_active
		  AND NOT EXISTS (
			  SELECT 1 FROM user_map_pin_unlocks u
			  WHERE u.user_id = $1 AND u.challenge_key = c.challenge_key
		  )
		ORDER BY c.display_order, c.challenge_key`, userID)
	if err != nil {
		return err
	}
	challenges := make([]unlockChallenge, 0)
	for rows.Next() {
		var item unlockChallenge
		var encoded []byte
		if err := rows.Scan(&item.key, &encoded); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(encoded, &item.criteria); err != nil {
			rows.Close()
			return fmt.Errorf("decode map pin challenge %q criteria: %w", item.key, err)
		}
		challenges = append(challenges, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, item := range challenges {
		eligible, err := r.challengeEligible(ctx, userID, item.criteria, now)
		if err != nil {
			return fmt.Errorf("evaluate map pin challenge %q: %w", item.key, err)
		}
		if eligible {
			if _, err := r.AwardMapPinUnlock(ctx, userID, item.key, now); err != nil {
				return fmt.Errorf("record map pin challenge %q: %w", item.key, err)
			}
		}
	}
	return nil
}

func (r *Repository) challengeEligible(ctx context.Context, userID string, criteria challengeCriteria, now time.Time) (bool, error) {
	switch criteria.Kind {
	case "single_guess_score":
		return r.scoreCount(ctx, userID, criteria, true)
	case "score_count":
		return r.scoreCount(ctx, userID, criteria, true)
	case "score_streak":
		return r.hasScoreStreak(ctx, userID, criteria)
	case "distance_count":
		return r.distanceCount(ctx, userID, criteria)
	case "unique_groups":
		if criteria.Count <= 0 {
			return false, errors.New("unique_groups requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT group_id) >= $2
			FROM guesses
			WHERE user_id = $1 AND NOT timed_out`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "timed_public_count":
		return r.timedPublicCount(ctx, userID, criteria)
	case "public_author_count":
		if criteria.Count <= 0 {
			return false, errors.New("public_author_count requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT p.user_id) >= $2
			FROM public_guesses g
			JOIN public_challenges p ON p.id = g.challenge_id
			WHERE g.user_id = $1 AND NOT g.timed_out AND p.audience = 'public'`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "received_public_guesses":
		if criteria.Count <= 0 {
			return false, errors.New("received_public_guesses requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) >= $2
			FROM public_guesses g
			JOIN public_challenges p ON p.id = g.challenge_id
			WHERE p.user_id = $1 AND p.audience = 'public' AND g.user_id <> p.user_id AND NOT g.timed_out`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "received_public_guessers":
		if criteria.Count <= 0 {
			return false, errors.New("received_public_guessers requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT g.user_id) >= $2
			FROM public_guesses g
			JOIN public_challenges p ON p.id = g.challenge_id
			WHERE p.user_id = $1 AND p.audience = 'public' AND g.user_id <> p.user_id AND NOT g.timed_out`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "received_public_reactions":
		if criteria.Count <= 0 {
			return false, errors.New("received_public_reactions requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) >= $2
			FROM public_reactions r
			JOIN public_challenges p ON p.id = r.challenge_id
			WHERE p.user_id = $1 AND p.audience = 'public' AND r.user_id <> p.user_id`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "authored_public_comments":
		if criteria.Count <= 0 {
			return false, errors.New("authored_public_comments requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) >= $2
			FROM public_comments c
			JOIN public_challenges p ON p.id = c.challenge_id
			WHERE c.user_id = $1 AND p.audience = 'public'`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "public_guess_count":
		if criteria.Count <= 0 {
			return false, errors.New("public_guess_count requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) >= $2
			FROM public_guesses g
			JOIN public_challenges p ON p.id = g.challenge_id
			WHERE g.user_id = $1 AND NOT g.timed_out AND p.audience = 'public'`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "utc_hour_points":
		if criteria.ThresholdExclusive <= 0 || criteria.StartUTCHour < 0 || criteria.StartUTCHour > 23 || criteria.EndUTCHour < 1 || criteria.EndUTCHour > 24 || criteria.StartUTCHour >= criteria.EndUTCHour {
			return false, errors.New("utc_hour_points requires a positive threshold and a valid UTC hour interval")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(score), 0) > $4
			FROM public_guesses
			WHERE user_id = $1 AND NOT timed_out
			  AND EXTRACT(HOUR FROM created_at AT TIME ZONE 'UTC') >= $2
		  AND EXTRACT(HOUR FROM created_at AT TIME ZONE 'UTC') < $3`, userID, criteria.StartUTCHour, criteria.EndUTCHour, criteria.ThresholdExclusive).Scan(&eligible)
		return eligible, err
	case "weekend_points":
		if criteria.ThresholdExclusive <= 0 || criteria.WeekStart != "monday_utc" {
			return false, errors.New("weekend_points requires a positive threshold and monday_utc weeks")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM (
					SELECT date_trunc('week', created_at AT TIME ZONE 'UTC') AS week_start, SUM(score)::BIGINT AS points
					FROM (
						SELECT score, created_at FROM guesses WHERE user_id = $1 AND NOT timed_out
						UNION ALL
						SELECT score, created_at FROM public_guesses WHERE user_id = $1 AND NOT timed_out
					) scored
					WHERE EXTRACT(ISODOW FROM created_at AT TIME ZONE 'UTC') IN (6, 7)
					GROUP BY date_trunc('week', created_at AT TIME ZONE 'UTC')
				) weekly
				WHERE weekly.points >= $2
			)`, userID, criteria.ThresholdExclusive).Scan(&eligible)
		return eligible, err
	case "total_points":
		if criteria.ThresholdExclusive <= 0 {
			return false, errors.New("total_points requires a positive threshold")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(score), 0) >= $2
			FROM (
				SELECT score FROM guesses WHERE user_id = $1 AND NOT timed_out
				UNION ALL
				SELECT score FROM public_guesses WHERE user_id = $1 AND NOT timed_out
			) scored`, userID, criteria.ThresholdExclusive).Scan(&eligible)
		return eligible, err
	case "source_score_count":
		return r.sourceScoreCount(ctx, userID, criteria)
	case "public_reaction_count":
		if criteria.Count <= 0 {
			return false, errors.New("public_reaction_count requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT r.challenge_id) >= $2
			FROM public_reactions r
			JOIN public_challenges p ON p.id = r.challenge_id
			WHERE r.user_id = $1 AND p.audience = 'public' AND p.user_id <> r.user_id`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "public_commented_challenges":
		if criteria.Count <= 0 {
			return false, errors.New("public_commented_challenges requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT c.challenge_id) >= $2
			FROM public_comments c
			JOIN public_challenges p ON p.id = c.challenge_id
			WHERE c.user_id = $1 AND p.audience = 'public'`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "timed_public_speed":
		return r.timedPublicCount(ctx, userID, criteria)
	case "group_weekly_rank":
		if criteria.Rank != 1 || criteria.TiePolicy != "shared_first" || criteria.WeekStart != "monday_utc" {
			return false, errors.New("group_weekly_rank requires rank 1, shared_first ties, and monday_utc weeks")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			WITH weekly AS (
				SELECT g.group_id, g.user_id,
				       date_trunc('week', g.created_at AT TIME ZONE 'UTC') AS week_start,
				       SUM(g.score)::BIGINT AS points
				FROM guesses g
				JOIN group_members gm ON gm.group_id = g.group_id AND gm.user_id = g.user_id
				JOIN users u ON u.id = g.user_id AND u.deleted_at IS NULL
				WHERE NOT g.timed_out AND g.created_at < $2
				GROUP BY g.group_id, g.user_id, date_trunc('week', g.created_at AT TIME ZONE 'UTC')
			)
			SELECT EXISTS (
				SELECT 1 FROM weekly mine
				WHERE mine.user_id = $1
				  AND mine.week_start < date_trunc('week', $2::timestamptz AT TIME ZONE 'UTC')
				  AND mine.points = (
					  SELECT MAX(competitor.points) FROM weekly competitor
					  WHERE competitor.group_id = mine.group_id AND competitor.week_start = mine.week_start
				  )
			)`, userID, now).Scan(&eligible)
		return eligible, err
	case "weekly_points":
		if criteria.ThresholdExclusive < 0 || criteria.WeekStart != "monday_utc" {
			return false, errors.New("weekly_points requires a non-negative threshold and monday_utc weeks")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM (
					SELECT date_trunc('week', created_at AT TIME ZONE 'UTC') AS week_start, SUM(score)::BIGINT AS points
					FROM (
						SELECT score, created_at FROM guesses WHERE user_id = $1 AND NOT timed_out AND created_at < $3
						UNION ALL
						SELECT score, created_at FROM public_guesses WHERE user_id = $1 AND NOT timed_out AND created_at < $3
					) scored
					GROUP BY date_trunc('week', created_at AT TIME ZONE 'UTC')
				) weekly
				WHERE weekly.points > $2
			)`, userID, criteria.ThresholdExclusive, now).Scan(&eligible)
		return eligible, err
	case "published_public_locations":
		if criteria.Count <= 0 {
			return false, errors.New("published_public_locations requires a positive count")
		}
		var eligible bool
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT (lat, long)) >= $2
			FROM public_challenges
			WHERE user_id = $1 AND audience = 'public'`, userID, criteria.Count).Scan(&eligible)
		return eligible, err
	case "country_score_count":
		if criteria.MinimumScore < 0 || criteria.Countries <= 0 {
			return false, errors.New("country_score_count requires a non-negative score and positive country count")
		}
		return r.hasScoresInEnoughCountries(ctx, userID, criteria.MinimumScore, criteria.Countries)
	case "distance_score_count":
		if criteria.MinimumScore <= 0 {
			return false, errors.New("distance_score_count requires a positive minimum score")
		}
		return r.distanceCount(ctx, userID, criteria)
	default:
		// Unknown criteria remain visible as locked catalog entries until the
		// server adds an evaluator for them.
		return false, nil
	}
}
