package feed

import (
	"context"
	"time"

	"geoguessme/internal/elo"
	"geoguessme/internal/models"
)

// Results returns every completed public guess for a visible challenge. Elo
// deltas replay the combined private/public history with the same stable
// all-time factor used by the global ladder.
func (r *Repository) Results(ctx context.Context, id, viewer string) ([]models.PublicFeedResult, error) {
	var visible bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public_challenges p WHERE p.id=$2 AND `+challengeVisibility+`)`, viewer, id).Scan(&visible); err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT g.user_id,u.username,u.avatar,g.score,g.distance,
		COALESCE(mp.pin_key, ''), COALESCE(mp.name, ''), COALESCE(mp.image_url, '')
		FROM public_guesses g
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		JOIN public_challenges p ON p.id=g.challenge_id
		LEFT JOIN user_equipped_map_pins ep ON ep.user_id=g.user_id
		LEFT JOIN map_pins mp ON mp.pin_key=ep.pin_key
		WHERE g.challenge_id=$2 AND `+challengeVisibility+` AND NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
			(b.blocker_id=$1 AND b.blocked_id=g.user_id) OR (b.blocker_id=g.user_id AND b.blocked_id=$1))
		ORDER BY g.score DESC,g.created_at ASC,g.user_id ASC`, viewer, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	global, err := r.globalChallenges(ctx)
	if err != nil {
		return nil, err
	}
	deltas := elo.ComputeChallengeDeltas(global, elo.FactorAllTime)["public:"+id]
	results := make([]models.PublicFeedResult, 0)
	for rows.Next() {
		var result models.PublicFeedResult
		var pinKey, pinName, pinImage string
		if err := rows.Scan(&result.UserID, &result.Username, &result.Avatar, &result.Score, &result.Distance, &pinKey, &pinName, &pinImage); err != nil {
			return nil, err
		}
		if pinKey != "" {
			result.MapPin = &models.MapPin{Key: pinKey, Name: pinName, ImageURL: pinImage}
		}
		result.Rank = len(results) + 1
		result.EloDelta = deltas[result.UserID]
		result.IsViewer = result.UserID == viewer
		results = append(results, result)
	}
	return results, rows.Err()
}

func (r *Repository) globalChallenges(ctx context.Context) ([]elo.Challenge, error) {
	rows, err := r.pool.Query(ctx, `SELECT challenge_id,created_at,user_id,score FROM (
		SELECT 'private:'||p.id AS challenge_id,p.created_at,g.user_id,g.score
		FROM guesses g
		JOIN photos p ON p.id=g.photo_id
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		WHERE NOT g.timed_out
		UNION ALL
		SELECT 'public:'||p.id AS challenge_id,p.created_at,g.user_id,g.score
		FROM public_guesses g
		JOIN public_challenges p ON p.id=g.challenge_id
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
	) history ORDER BY created_at,challenge_id,user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byChallenge := map[string]*elo.Challenge{}
	var order []string
	for rows.Next() {
		var id, userID string
		var createdAt time.Time
		var score int
		if err := rows.Scan(&id, &createdAt, &userID, &score); err != nil {
			return nil, err
		}
		challenge, ok := byChallenge[id]
		if !ok {
			challenge = &elo.Challenge{ID: id, CreatedAt: createdAt}
			byChallenge[id] = challenge
			order = append(order, id)
		}
		challenge.Guesses = append(challenge.Guesses, elo.Guess{UserID: userID, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	challenges := make([]elo.Challenge, 0, len(order))
	for _, id := range order {
		if challenge := byChallenge[id]; len(challenge.Guesses) >= 2 {
			challenges = append(challenges, *challenge)
		}
	}
	return challenges, nil
}
