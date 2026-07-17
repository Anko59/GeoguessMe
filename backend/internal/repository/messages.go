package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func SaveMessage(msg *models.Message) error {
	return SaveMessageContext(context.Background(), msg)
}

func SaveMessageContext(ctx context.Context, msg *models.Message) error {
	if msg.Username == "" {
		var username, avatar string
		if err := database.DB.QueryRow(ctx, `SELECT username, avatar FROM users WHERE id = $1`, msg.UserID).Scan(&username, &avatar); err == nil {
			msg.Username, msg.Avatar = username, avatar
		}
	}
	_, err := database.DB.Exec(ctx, `INSERT INTO messages(id, group_id, user_id, kind, photo_id, content, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, msg.ID, msg.GroupID, msg.UserID, msg.Kind, msg.PhotoID, msg.Content, msg.CreatedAt)
	return err
}

const messageColumns = "m.id, m.group_id, m.user_id, u.username, u.avatar, m.kind, m.photo_id, m.content, m.created_at"

// MessagesPage is the cursor-paginated result of GetGroupMessagesPage.
type MessagesPage struct {
	Items      []models.Message `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

// GetGroupMessagesPage returns messages strictly after the opaque cursor,
// ordered by the stable tuple (created_at, id). An empty cursor returns the
// most recent page. The returned next_cursor is empty when no more pages exist.
func GetGroupMessagesPage(ctx context.Context, groupID, cursor string, limit int) (MessagesPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	args := []any{groupID}
	query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id WHERE m.group_id = $1`
	if cursor != "" {
		createdAt, id, err := decodeMessageCursor(cursor)
		if err != nil {
			return MessagesPage{}, fmt.Errorf("invalid message cursor: %w", err)
		}
		args = append(args, createdAt, id)
		query += ` AND ROW(m.created_at, m.id) > ROW($2, $3)`
	}
	args = append(args, limit+1)
	query += fmt.Sprintf(` ORDER BY m.created_at ASC, m.id ASC LIMIT $%d`, len(args))

	rows, err := database.DB.Query(ctx, query, args...)
	if err != nil {
		return MessagesPage{}, err
	}
	defer rows.Close()
	messages := make([]models.Message, 0, limit)
	for rows.Next() {
		var msg models.Message
		var username, avatar sql.NullString
		if err := rows.Scan(&msg.ID, &msg.GroupID, &msg.UserID, &username, &avatar, &msg.Kind, &msg.PhotoID, &msg.Content, &msg.CreatedAt); err != nil {
			return MessagesPage{}, err
		}
		if username.Valid {
			msg.Username = username.String
		}
		if avatar.Valid {
			msg.Avatar = avatar.String
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return MessagesPage{}, err
	}

	page := MessagesPage{Items: messages}
	if len(messages) > limit {
		last := messages[limit-1]
		page.Items = messages[:limit]
		page.NextCursor = encodeMessageCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// GetGroupMessagesContext remains as a thin cursor wrapper for any caller that
// still passes a legacy message id. New callers should use the page API.
func GetGroupMessagesContext(ctx context.Context, groupID, afterID string) ([]models.Message, error) {
	cursor := ""
	if afterID != "" {
		var createdAt time.Time
		err := database.DB.QueryRow(ctx, `SELECT created_at FROM messages WHERE id = $1`, afterID).Scan(&createdAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			cursor = encodeMessageCursor(createdAt, afterID)
		}
	}
	page, err := GetGroupMessagesPage(ctx, groupID, cursor, 500)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func GetGroupMessages(groupID string) ([]models.Message, error) {
	return GetGroupMessagesContext(context.Background(), groupID, "")
}

func encodeMessageCursor(createdAt time.Time, id string) string {
	payload := strconv.FormatInt(createdAt.UnixNano(), 10) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeMessageCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	if parts[1] == "" {
		return time.Time{}, "", errors.New("malformed cursor id")
	}
	return time.Unix(0, nanos).UTC(), parts[1], nil
}

// Ensure the explicit timestamp is always initialized by server code.
func NewTextMessage(groupID, userID, content string, now time.Time) *models.Message {
	return &models.Message{ID: uuid.NewString(), GroupID: groupID, UserID: userID, Kind: "text", Content: content, CreatedAt: now}
}
