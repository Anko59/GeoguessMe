package repository

import (
	"context"
	"errors"

	"geoguessme/internal/database"
	"geoguessme/internal/models"

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
	defer tx.Rollback(ctx)
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

type LeaderboardEntry struct {
	UserID     string  `json:"user_id"`
	Username   string  `json:"username"`
	Score      int     `json:"score"`
	GuessCount int     `json:"guess_count"`
	Average    float64 `json:"average_score"`
}

func GetGroupLeaderboard(groupID string) ([]LeaderboardEntry, error) {
	return GetGroupLeaderboardContext(context.Background(), groupID)
}

func GetGroupLeaderboardContext(ctx context.Context, groupID string) ([]LeaderboardEntry, error) {
	query := `
		SELECT u.id, u.username,
		       COALESCE(CAST(AVG(g.score) AS INTEGER), 0),
		       COUNT(g.id), COALESCE(AVG(g.score), 0)
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id AND u.deleted_at IS NULL
		LEFT JOIN guesses g ON g.user_id = u.id AND g.group_id = gm.group_id
		WHERE gm.group_id = $1
		GROUP BY u.id, u.username
		ORDER BY COALESCE(AVG(g.score), 0) DESC, COUNT(g.id) DESC, u.username ASC`
	rows, err := database.DB.Query(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.Score, &entry.GuessCount, &entry.Average); err != nil {
			return nil, err
		}
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
