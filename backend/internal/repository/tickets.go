package repository

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"

	"github.com/jackc/pgx/v5"
)

func CreateWebSocketTicket(ctx context.Context, id, userID, groupID, tokenHash string, expiresAt time.Time) error {
	_, err := database.DB.Exec(ctx, `INSERT INTO websocket_tickets(id, user_id, group_id, token_hash, expires_at) VALUES ($1, $2, $3, $4, $5)`, id, userID, groupID, tokenHash, expiresAt)
	return err
}

func ConsumeWebSocketTicket(ctx context.Context, tokenHash, groupID string) (userID string, err error) {
	err = database.DB.QueryRow(ctx, `UPDATE websocket_tickets SET used_at = CURRENT_TIMESTAMP WHERE token_hash = $1 AND group_id = $2 AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP RETURNING user_id`, tokenHash, groupID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}
