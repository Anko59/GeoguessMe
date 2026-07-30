package repository

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/models"
	"geoguessme/internal/progression"

	"github.com/jackc/pgx/v5"
)

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
	Rank        progression.Rank `json:"rank"`
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
	return result, rows.Err()
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

func GetUserGroups(userID string) ([]models.Group, error) {
	return GetUserGroupsContext(context.Background(), userID)
}

func GetUserGroupsContext(ctx context.Context, userID string) ([]models.Group, error) {
	rows, err := database.DB.Query(ctx, `SELECT g.id, g.name, g.code, g.created_at FROM groups g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_id = $1 ORDER BY g.created_at DESC`, userID)
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
