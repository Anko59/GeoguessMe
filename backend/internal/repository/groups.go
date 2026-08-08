package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/elo"
	"geoguessme/internal/models"
	"geoguessme/internal/progression"
	"geoguessme/internal/repository/chat"

	"github.com/jackc/pgx/v5"
)

// Repository is the concrete PostgreSQL persistence collection. PR 4
// introduces it as the dependency-injected seam: each migrated slice becomes a
// method on a repository bound to the injected pool, and the matching
// package-level function that read the database.DB global is removed. Slices
// not yet migrated still use the package-level functions. The chat slice was
// split into its own sub-package (Chat) in PR 5 because this directory is at
// its structural file limit; PR 6 continues the split for groups and
// leaderboards.
//
// Instances are independent: two Repositories built on different pools never
// share state.
type Repository struct {
	pool database.Pool
	// Chat is the chat slice's persistence collection (messages, reactions,
	// chat media, and WebSocket tickets). The application composition root
	// hands it to the ChatAPI through App.Repos.Chat.
	Chat *chat.Repository
}

// NewRepository returns a Repository bound to the given pool, including the
// chat persistence slice.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool, Chat: chat.NewRepository(pool)}
}

func CreateGroup(group *models.Group) error {
	return CreateGroupAndMembership(context.Background(), group, "")
}

func CreateGroupAndMembership(ctx context.Context, group *models.Group, userID string) error {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO groups (id, name, code, created_at) VALUES ($1, $2, $3, $4)`, group.ID, group.Name, group.Code, group.CreatedAt); err != nil {
		return err
	}
	if userID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3)`, group.ID, userID, group.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func GetGroupByCode(code string) (*models.Group, error) {
	return GetGroupByCodeContext(context.Background(), code)
}

func GetGroupByCodeContext(ctx context.Context, code string) (*models.Group, error) {
	var group models.Group
	err := database.DB.QueryRow(ctx, `SELECT id, name, code, created_at FROM groups WHERE code = $1`, code).Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &group, err
}

func AddGroupMember(member *models.GroupMember) error {
	_, err := database.DB.Exec(context.Background(), `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3)`, member.GroupID, member.UserID, member.JoinedAt)
	return err
}

func AddGroupMemberContext(ctx context.Context, member *models.GroupMember) error {
	_, err := database.DB.Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, member.GroupID, member.UserID, member.JoinedAt)
	return err
}

func IsGroupMember(groupID, userID string) (bool, error) {
	return IsGroupMemberContext(context.Background(), groupID, userID)
}

func IsGroupMemberContext(ctx context.Context, groupID, userID string) (bool, error) {
	var exists bool
	err := database.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, groupID, userID).Scan(&exists)
	return exists, err
}

// SharesGroupContext reports whether two users are members of at least one
// common group. A user always shares a group with themself. It backs the
// player profile endpoint, which is only visible to players who actually
// interact with the target in a group.
func SharesGroupContext(ctx context.Context, userA, userB string) (bool, error) {
	if userA == userB {
		return true, nil
	}
	var shared bool
	err := database.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM group_members a
			JOIN group_members b ON b.group_id = a.group_id
			WHERE a.user_id = $1 AND b.user_id = $2
		)`, userA, userB).Scan(&shared)
	return shared, err
}

func GetGroupPhotoContext(ctx context.Context, groupID string) (*models.GroupPhoto, error) {
	var photo models.GroupPhoto
	err := database.DB.QueryRow(ctx, `SELECT group_id, storage_key, mime_type, byte_size, created_at FROM group_photos WHERE group_id = $1`, groupID).Scan(&photo.GroupID, &photo.StorageKey, &photo.MIMEType, &photo.ByteSize, &photo.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &photo, err
}

// SetGroupPhoto atomically points a group at a newly stored photo and returns
// the previous storage key so callers can retire the replaced object after the
// database commit succeeds.
func SetGroupPhotoContext(ctx context.Context, photo *models.GroupPhoto) (string, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM group_photos WHERE group_id = $1 FOR UPDATE`, photo.GroupID).Scan(&previousKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO group_photos (group_id, storage_key, mime_type, byte_size, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id) DO UPDATE SET storage_key = EXCLUDED.storage_key, mime_type = EXCLUDED.mime_type, byte_size = EXCLUDED.byte_size, created_at = EXCLUDED.created_at`, photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return previousKey, nil
}

func GetGroupNotificationPreferenceContext(ctx context.Context, groupID, userID string) (bool, error) {
	var enabled bool
	err := database.DB.QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM group_notification_preferences WHERE group_id = $1 AND user_id = $2), TRUE)`, groupID, userID).Scan(&enabled)
	return enabled, err
}

func SetGroupNotificationPreferenceContext(ctx context.Context, groupID, userID string, enabled bool) error {
	_, err := database.DB.Exec(ctx, `INSERT INTO group_notification_preferences (group_id, user_id, enabled, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (group_id, user_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = CURRENT_TIMESTAMP`, groupID, userID, enabled)
	return err
}

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

type LeaderboardMetric string

const (
	LeaderboardMetricTotal   LeaderboardMetric = "total"
	LeaderboardMetricAverage LeaderboardMetric = "average"
	LeaderboardMetricElo     LeaderboardMetric = "elo"
)

func ParseLeaderboardMetric(value string) (LeaderboardMetric, bool) {
	switch LeaderboardMetric(value) {
	case LeaderboardMetricTotal, LeaderboardMetricAverage, LeaderboardMetricElo:
		return LeaderboardMetric(value), true
	default:
		return "", false
	}
}

type LeaderboardPeriod string

const (
	LeaderboardAllTime LeaderboardPeriod = "all"
	LeaderboardWeek    LeaderboardPeriod = "week"
	LeaderboardMonth   LeaderboardPeriod = "month"
)

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

func GetGroupLeaderboard(groupID string) ([]LeaderboardEntry, error) {
	return GetGroupLeaderboardContext(context.Background(), groupID)
}

func GetGroupLeaderboardContext(ctx context.Context, groupID string) ([]LeaderboardEntry, error) {
	return GetGroupLeaderboardForPeriodContext(ctx, groupID, LeaderboardAllTime)
}

func GetGroupLeaderboardForPeriodContext(ctx context.Context, groupID string, period LeaderboardPeriod) ([]LeaderboardEntry, error) {
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
	rows, err := database.DB.Query(ctx, query, args...)
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
	// Elo ratings come from pairwise comparisons on the period's challenges;
	// they are recomputed here so a late guess on an old challenge moves the
	// whole period ladder, not just one row.
	challenges, err := LoadChallengesContext(ctx, groupID, start)
	if err != nil {
		return nil, err
	}
	ratings := elo.ComputeRatings(challenges)
	for index := range result {
		result[index].Elo = ratings[result[index].UserID]
	}
	return result, nil
}

func GetGroupByID(groupID string) (*models.Group, error) {
	return GetGroupByIDContext(context.Background(), groupID)
}

func GetGroupByIDContext(ctx context.Context, groupID string) (*models.Group, error) {
	var group models.Group
	err := database.DB.QueryRow(ctx, `SELECT id, name, code, created_at FROM groups WHERE id = $1`, groupID).Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &group, err
}

// UserGroups returns the groups a user belongs to, newest first. It is the
// read-only pilot slice (PR 4) migrated onto the injected repository seam; the
// previous package-level GetUserGroups and GetUserGroupsContext were removed
// so the query has a single implementation.
func (r *Repository) UserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id, g.name, g.code, g.created_at FROM groups g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_id = $1 ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.Group
	for rows.Next() {
		var group models.Group
		if err := rows.Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// LoadChallengesContext returns every challenge (photo with guesses) for a
// group, optionally limited to photos created at or after start, ordered by
// photo creation time. Challenges with fewer than two guessers are dropped:
// they produce no Elo comparisons. Guesses by deleted accounts are excluded.
func LoadChallengesContext(ctx context.Context, groupID string, start *time.Time) ([]elo.Challenge, error) {
	return loadChallengesContext(ctx, &groupID, start)
}

// LoadGlobalChallengesContext returns every challenge across all groups,
// ordered by photo creation time. It is the input for the global Elo ladder.
func LoadGlobalChallengesContext(ctx context.Context) ([]elo.Challenge, error) {
	return loadChallengesContext(ctx, nil, nil)
}

func loadChallengesContext(ctx context.Context, groupID *string, start *time.Time) ([]elo.Challenge, error) {
	query := `
		SELECT p.id, p.created_at, g.user_id, g.score
		FROM guesses g
		JOIN photos p ON p.id = g.photo_id
		JOIN users u ON u.id = g.user_id AND u.deleted_at IS NULL
		WHERE TRUE`
	args := []any{}
	if groupID != nil {
		query += ` AND g.group_id = $1`
		args = append(args, *groupID)
	}
	if start != nil {
		query += ` AND p.created_at >= $2`
		args = append(args, *start)
	}
	query += ` ORDER BY p.created_at, p.id, g.user_id`

	rows, err := database.DB.Query(ctx, query, args...)
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

// GetGlobalAverageRankContext ranks the player by average guess score among
// every player with at least one guess.
func GetGlobalAverageRankContext(ctx context.Context, userID string) (GlobalRankStats, error) {
	var rank, totalPlayers int64
	err := database.DB.QueryRow(ctx, `
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

// GetGlobalEloContext recomputes the global Elo ladder and returns the
// player's rating and competition rank. A player with no qualifying challenge
// (never compared against anyone) is unrated: Elo 0, rank 0.
func GetGlobalEloContext(ctx context.Context, userID string) (EloStats, error) {
	challenges, err := LoadGlobalChallengesContext(ctx)
	if err != nil {
		return EloStats{}, err
	}
	ratings := elo.ComputeRatings(challenges)
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

// SortLeaderboard orders leaderboard entries by the requested metric, with
// total points and username as stable tie-breakers.
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
