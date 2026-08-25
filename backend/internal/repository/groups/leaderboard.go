package groups

import (
	"context"
	"fmt"
	"sort"
	"time"

	"geoguessme/internal/elo"
	"geoguessme/internal/progression"
)

// LeaderboardEntry is one row of a group leaderboard.
type LeaderboardEntry struct {
	UserID      string           `json:"user_id"`
	Username    string           `json:"username"`
	Avatar      string           `json:"avatar"`
	Score       int              `json:"score"`
	GuessCount  int              `json:"guess_count"`
	Average     float64          `json:"average_score"`
	TotalPoints int              `json:"total_points"`
	Elo         int              `json:"elo"`
	Rank        progression.Rank `json:"rank"`
}

// LeaderboardMetric selects the ordering metric for a leaderboard.
type LeaderboardMetric string

const (
	LeaderboardMetricTotal   LeaderboardMetric = "total"
	LeaderboardMetricAverage LeaderboardMetric = "average"
	LeaderboardMetricElo     LeaderboardMetric = "elo"
)

// ParseLeaderboardMetric validates a metric query value; an invalid value
// yields ok=false.
func ParseLeaderboardMetric(value string) (LeaderboardMetric, bool) {
	switch LeaderboardMetric(value) {
	case LeaderboardMetricTotal, LeaderboardMetricAverage, LeaderboardMetricElo:
		return LeaderboardMetric(value), true
	default:
		return "", false
	}
}

// LeaderboardPeriod selects the time window for a leaderboard.
type LeaderboardPeriod string

const (
	LeaderboardAllTime LeaderboardPeriod = "all"
	LeaderboardWeek    LeaderboardPeriod = "week"
	LeaderboardMonth   LeaderboardPeriod = "month"
)

// ParseLeaderboardPeriod validates a period query value; an invalid value
// yields ok=false.
func ParseLeaderboardPeriod(value string) (LeaderboardPeriod, bool) {
	switch LeaderboardPeriod(value) {
	case LeaderboardAllTime, LeaderboardWeek, LeaderboardMonth:
		return LeaderboardPeriod(value), true
	default:
		return "", false
	}
}

func leaderboardPeriodStart(period LeaderboardPeriod, now time.Time) *time.Time {
	now = now.UTC()
	switch period {
	case LeaderboardWeek:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
		return &start
	case LeaderboardMonth:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return &start
	default:
		return nil
	}
}

// leaderboardFactor maps a leaderboard period to its Elo update factor: the
// weekly ladder moves fastest to reward current form, the monthly ladder is
// moderate, and the all-time ladder moves slowest so it reflects durable
// skill instead of the most recent challenges.
func leaderboardFactor(period LeaderboardPeriod) elo.Factor {
	switch period {
	case LeaderboardWeek:
		return elo.FactorWeekly
	case LeaderboardMonth:
		return elo.FactorMonthly
	default:
		return elo.FactorAllTime
	}
}

// LeaderboardForPeriod returns the leaderboard of a group for a period. Elo
// ratings come from pairwise comparisons on the period's challenges and are
// recomputed here so a late guess on an old challenge moves the whole period
// ladder, not just one row; the period also selects the update factor (see
// leaderboardFactor). Ranking computation stays pure: SortLeaderboard orders
// the rows and elo.ComputeRatings is a pure function over the loaded
// challenges.
func (r *Repository) LeaderboardForPeriod(ctx context.Context, groupID string, period LeaderboardPeriod) ([]LeaderboardEntry, error) {
	start := leaderboardPeriodStart(period, time.Now())
	query := `
		SELECT u.id, u.username, u.avatar,
		       COALESCE(SUM(g.score), 0),
		       COUNT(g.id), COALESCE(AVG(g.score), 0),
		       COALESCE(all_time.total_points, 0)
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id AND u.deleted_at IS NULL
		LEFT JOIN guesses g ON g.user_id = u.id AND g.group_id = gm.group_id
		LEFT JOIN (
			SELECT user_id, SUM(score) AS total_points
			FROM guesses
			GROUP BY user_id
		) all_time ON all_time.user_id = u.id
		WHERE gm.group_id = $1
		GROUP BY u.id, u.username, u.avatar, all_time.total_points
		ORDER BY COALESCE(SUM(g.score), 0) DESC, COUNT(g.id) DESC, u.username ASC`
	args := []any{groupID}
	if start != nil {
		query = `
			SELECT u.id, u.username, u.avatar,
			       COALESCE(SUM(g.score), 0),
			       COUNT(g.id), COALESCE(AVG(g.score), 0),
			       COALESCE(all_time.total_points, 0)
			FROM group_members gm
			JOIN users u ON gm.user_id = u.id AND u.deleted_at IS NULL
			LEFT JOIN guesses g ON g.user_id = u.id AND g.group_id = gm.group_id AND g.created_at >= $2
			LEFT JOIN (
				SELECT user_id, SUM(score) AS total_points
				FROM guesses
				GROUP BY user_id
			) all_time ON all_time.user_id = u.id
			WHERE gm.group_id = $1
			GROUP BY u.id, u.username, u.avatar, all_time.total_points
			ORDER BY COALESCE(SUM(g.score), 0) DESC, COUNT(g.id) DESC, u.username ASC`
		args = append(args, *start)
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.Avatar, &entry.Score, &entry.GuessCount, &entry.Average, &entry.TotalPoints); err != nil {
			return nil, err
		}
		entry.Rank = progression.RankForPoints(entry.TotalPoints)
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	challenges, err := r.LoadChallenges(ctx, groupID, start)
	if err != nil {
		return nil, err
	}
	ratings := elo.ComputeRatings(challenges, leaderboardFactor(period))
	for index := range result {
		result[index].Elo = ratings[result[index].UserID]
	}
	return result, nil
}

// SortLeaderboard orders leaderboard entries by the requested metric, with
// total points and username as stable tie-breakers. It is pure: it only
// reorders the rows it is given.
func SortLeaderboard(entries []LeaderboardEntry, metric LeaderboardMetric) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		switch metric {
		case LeaderboardMetricAverage:
			if a.Average != b.Average {
				return a.Average > b.Average
			}
		case LeaderboardMetricElo:
			if a.Elo != b.Elo {
				return a.Elo > b.Elo
			}
		default:
			if a.Score != b.Score {
				return a.Score > b.Score
			}
		}
		if a.TotalPoints != b.TotalPoints {
			return a.TotalPoints > b.TotalPoints
		}
		return a.Username < b.Username
	})
}

// Challenge is a photo with its guesses, the Elo input for one comparison
// round. It aliases elo.Challenge so callers of this package never need to
// import the elo package.
type Challenge = elo.Challenge

// LoadChallenges returns every challenge (photo with guesses) for a group,
// optionally limited to photos created at or after start, ordered by photo
// creation time. Challenges with fewer than two guessers are dropped: they
// produce no Elo comparisons. Guesses by deleted accounts are excluded.
func (r *Repository) LoadChallenges(ctx context.Context, groupID string, start *time.Time) ([]elo.Challenge, error) {
	return r.loadChallenges(ctx, &groupID, start)
}

// GlobalChallenges returns every challenge across all groups, ordered by
// photo creation time. It is the input for the global Elo ladder.
func (r *Repository) GlobalChallenges(ctx context.Context) ([]elo.Challenge, error) {
	return r.loadChallenges(ctx, nil, nil)
}

func (r *Repository) loadChallenges(ctx context.Context, groupID *string, start *time.Time) ([]elo.Challenge, error) {
	query := `
		SELECT p.id, p.created_at, g.user_id, g.score
		FROM guesses g
		JOIN photos p ON p.id = g.photo_id
		JOIN users u ON u.id = g.user_id AND u.deleted_at IS NULL
		WHERE TRUE`
	// Placeholders are numbered by position so every filter combination is
	// valid SQL: the time filter is $2 only when the group filter precedes
	// it, and $1 when it is the sole argument.
	args := []any{}
	if groupID != nil {
		args = append(args, *groupID)
		query += fmt.Sprintf(` AND g.group_id = $%d`, len(args))
	}
	if start != nil {
		args = append(args, *start)
		query += fmt.Sprintf(` AND p.created_at >= $%d`, len(args))
	}
	query += ` ORDER BY p.created_at, p.id, g.user_id`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byPhoto := map[string]*elo.Challenge{}
	var order []string
	for rows.Next() {
		var photoID, userID string
		var createdAt time.Time
		var score int
		if err := rows.Scan(&photoID, &createdAt, &userID, &score); err != nil {
			return nil, err
		}
		challenge, ok := byPhoto[photoID]
		if !ok {
			challenge = &elo.Challenge{ID: photoID, CreatedAt: createdAt}
			byPhoto[photoID] = challenge
			order = append(order, photoID)
		}
		challenge.Guesses = append(challenge.Guesses, elo.Guess{UserID: userID, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	challenges := make([]elo.Challenge, 0, len(order))
	for _, photoID := range order {
		challenge := byPhoto[photoID]
		if len(challenge.Guesses) >= 2 {
			challenges = append(challenges, *challenge)
		}
	}
	return challenges, nil
}

// GlobalRankStats is the player's rank by a numeric statistic among all
// players.
type GlobalRankStats struct {
	Rank         int
	TotalPlayers int
}

// GlobalAverageRank ranks the player by average guess score among every player
// with at least one guess.
func (r *Repository) GlobalAverageRank(ctx context.Context, userID string) (GlobalRankStats, error) {
	var rank, totalPlayers int64
	err := r.pool.QueryRow(ctx, `
		WITH scores AS (
			SELECT g.user_id, AVG(g.score) AS value
			FROM guesses g
			JOIN users u ON u.id = g.user_id AND u.deleted_at IS NULL
			GROUP BY g.user_id
		), ranked AS (
			SELECT user_id, RANK() OVER (ORDER BY value DESC) AS r
			FROM scores
		)
		SELECT COALESCE(MAX(r) FILTER (WHERE user_id = $1), 0), COUNT(*)
		FROM ranked`, userID).Scan(&rank, &totalPlayers)
	if err != nil {
		return GlobalRankStats{}, err
	}
	return GlobalRankStats{Rank: int(rank), TotalPlayers: int(totalPlayers)}, nil
}

// EloStats is the player's global Elo rating and competition rank among every
// rated player.
type EloStats struct {
	Elo          int
	Rank         int
	TotalPlayers int
}

// GlobalElo recomputes the global all-time Elo ladder and returns the
// player's rating and competition rank among every rated player. The all-time
// factor keeps the global rating slow-moving so it measures long-run skill.
// A player with no qualifying challenge (never compared against anyone) is
// unrated: Elo 0, rank 0.
func (r *Repository) GlobalElo(ctx context.Context, userID string) (EloStats, error) {
	challenges, err := r.GlobalChallenges(ctx)
	if err != nil {
		return EloStats{}, err
	}
	ratings := elo.ComputeRatings(challenges, elo.FactorAllTime)
	myElo, rated := ratings[userID]
	if !rated {
		return EloStats{Elo: 0, Rank: 0, TotalPlayers: len(ratings)}, nil
	}
	rank := 1
	for _, rating := range ratings {
		if rating > myElo {
			rank++
		}
	}
	return EloStats{Elo: myElo, Rank: rank, TotalPlayers: len(ratings)}, nil
}

// WeeklyChallengeEloDeltas returns the weekly Elo rating change for each
// participant on a challenge: the all-group chronological progression limited
// to challenges created in the current calendar week and driven by the weekly
// update factor, matching the weekly ladders. A photo created before the week
// started contributes no weekly change, so the returned map is nil; the same
// holds for challenges with fewer than two guesses.
func (r *Repository) WeeklyChallengeEloDeltas(ctx context.Context, photoID string) (map[string]int, error) {
	start := leaderboardPeriodStart(LeaderboardWeek, time.Now())
	challenges, err := r.loadChallenges(ctx, nil, start)
	if err != nil {
		return nil, err
	}
	deltas := elo.ComputeChallengeDeltas(challenges, elo.FactorWeekly)
	return deltas[photoID], nil
}
