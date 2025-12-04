package repository

import (
	"context"
	"geoguessme/internal/database"
)

func GetGroupMembers(groupID string) ([]map[string]interface{}, error) {
	query := `
		SELECT u.id, u.username, u.avatar
		FROM users u
		JOIN group_members gm ON u.id = gm.user_id
		WHERE gm.group_id = $1
		ORDER BY u.username
	`

	rows, err := database.DB.Query(context.Background(), query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var id, username, avatar string
		if err := rows.Scan(&id, &username, &avatar); err != nil {
			return nil, err
		}

		members = append(members, map[string]interface{}{
			"id":       id,
			"username": username,
			"avatar":   avatar,
		})
	}

	return members, nil
}
