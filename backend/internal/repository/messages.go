package repository

import (
	"context"
	"database/sql"
	"geoguessme/internal/database"
	"geoguessme/internal/models"
)

func SaveMessage(msg *models.Message) error {
	query := `INSERT INTO messages (id, group_id, user_id, content, created_at) VALUES ($1, $2, $3, $4, $5)`
	_, err := database.DB.Exec(context.Background(), query, msg.ID, msg.GroupID, msg.UserID, msg.Content, msg.CreatedAt)
	return err
}

func GetGroupMessages(groupID string) ([]models.Message, error) {
	query := `
		SELECT m.id, m.group_id, m.user_id, u.username, u.avatar, m.content, m.created_at
		FROM messages m
		LEFT JOIN users u ON m.user_id = u.id
		WHERE m.group_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := database.DB.Query(context.Background(), query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		var username, avatar sql.NullString
		if err := rows.Scan(&msg.ID, &msg.GroupID, &msg.UserID, &username, &avatar, &msg.Content, &msg.CreatedAt); err != nil {
			// If there's an error scanning a row, we can log it and continue or return the error.
			// The instruction says `continue`, so we'll follow that.
			// However, typically you'd want to return the error here.
			// For now, following the instruction's `continue`.
			continue
		}
		if username.Valid {
			msg.Username = username.String
		}
		if avatar.Valid {
			msg.Avatar = avatar.String
		}
		messages = append(messages, msg)
	}
	// Check for any errors that occurred during iteration
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}
