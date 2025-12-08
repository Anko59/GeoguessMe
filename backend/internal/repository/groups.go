package repository

import (
	"context"
	"errors"
	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

func CreateGroup(group *models.Group) error {
	query := `INSERT INTO groups (id, name, code, created_at) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.Exec(context.Background(), query, group.ID, group.Name, group.Code, group.CreatedAt)
	return err
}

func GetGroupByCode(code string) (*models.Group, error) {
	query := `SELECT id, name, code, created_at FROM groups WHERE code = $1`
	row := database.DB.QueryRow(context.Background(), query, code)

	var group models.Group
	err := row.Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func AddGroupMember(member *models.GroupMember) error {
	query := `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3)`
	_, err := database.DB.Exec(context.Background(), query, member.GroupID, member.UserID, member.JoinedAt)
	return err
}

func IsGroupMember(groupID, userID string) (bool, error) {
	query := `SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2`
	var exists int
	err := database.DB.QueryRow(context.Background(), query, groupID, userID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type LeaderboardEntry struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Score    int    `json:"score"`
}

func GetGroupLeaderboard(groupID string) ([]LeaderboardEntry, error) {
	// Calculate average score from guesses for each user in the group
	query := `
		SELECT u.id, u.username, COALESCE(CAST(AVG(g.score) AS INTEGER), 0) as avg_score
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		LEFT JOIN guesses g ON u.id = g.user_id
		WHERE gm.group_id = $1
		GROUP BY u.id, u.username
		ORDER BY avg_score DESC
	`
	rows, err := database.DB.Query(context.Background(), query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaderboard []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.Score); err != nil {
			return nil, err
		}
		leaderboard = append(leaderboard, entry)
	}
	return leaderboard, nil
}

func GetGroupByID(groupID string) (*models.Group, error) {
	query := `SELECT id, name, code, created_at FROM groups WHERE id = $1`
	var group models.Group
	err := database.DB.QueryRow(context.Background(), query, groupID).Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func GetUserGroups(userID string) ([]models.Group, error) {
	query := `
		SELECT g.id, g.name, g.code, g.created_at
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		WHERE gm.user_id = $1
	`
	rows, err := database.DB.Query(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var group models.Group
		if err := rows.Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt); err != nil {
			continue
		}
		groups = append(groups, group)
	}
	return groups, nil
}
