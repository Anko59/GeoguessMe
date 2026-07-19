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

// GetGroupMessagesPage returns a page of group messages.
//
// An empty cursor selects the most recent page: the newest `limit` messages,
// returned in chronological (ascending) order, with an empty next_cursor
// because no newer pages exist. A non-empty opaque cursor returns the messages
// strictly after that cursor in ascending order; next_cursor is set when more
// pages remain and empty otherwise. Ordering always follows the stable tuple
// (created_at, id) so reconnect catch-up cannot skip or duplicate a message.
func GetGroupMessagesPage(ctx context.Context, groupID, cursor string, limit int) (MessagesPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	if cursor == "" {
		query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id WHERE m.group_id = $1 ORDER BY m.created_at DESC, m.id DESC LIMIT $2`
		rows, err := database.DB.Query(ctx, query, groupID, limit)
		if err != nil {
			return MessagesPage{}, err
		}
		messages, err := scanMessageRows(rows)
		if err != nil {
			return MessagesPage{}, err
		}
		// Fetch newest-first but expose the page in chronological order.
		reverseMessages(messages)
		return MessagesPage{Items: messages}, nil
	}

	createdAt, id, err := decodeMessageCursor(cursor)
	if err != nil {
		return MessagesPage{}, fmt.Errorf("invalid message cursor: %w", err)
	}
	query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id WHERE m.group_id = $1 AND ROW(m.created_at, m.id) > ROW($2, $3) ORDER BY m.created_at ASC, m.id ASC LIMIT $4`
	rows, err := database.DB.Query(ctx, query, groupID, createdAt, id, limit+1)
	if err != nil {
		return MessagesPage{}, err
	}
	messages, err := scanMessageRows(rows)
	if err != nil {
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

// GetGroupMessagesPageForViewer enriches challenge messages with state that is
// specific to the authenticated viewer. The state is derived from the
// existing challenge views and guesses tables, so reconnects and hard reloads
// restore the same action shown in the chat without client-only assumptions.
func GetGroupMessagesPageForViewer(ctx context.Context, groupID, cursor string, limit int, viewerID string) (MessagesPage, error) {
	page, err := GetGroupMessagesPage(ctx, groupID, cursor, limit)
	if err != nil || viewerID == "" || len(page.Items) == 0 {
		return page, err
	}
	photoIDs := make([]string, 0, len(page.Items))
	for _, message := range page.Items {
		if message.Kind == "challenge" && message.PhotoID != nil {
			photoIDs = append(photoIDs, *message.PhotoID)
		}
	}
	if len(photoIDs) == 0 {
		return page, nil
	}
	rows, err := database.DB.Query(ctx, `
		SELECT p.id,
			CASE
				WHEN p.user_id = $2 THEN 'results'
				WHEN EXISTS (SELECT 1 FROM guesses g WHERE g.photo_id = p.id AND g.user_id = $2) THEN 'guessed'
				WHEN p.expires_at <= NOW() THEN 'expired'
				WHEN EXISTS (SELECT 1 FROM challenge_views v WHERE v.photo_id = p.id AND v.user_id = $2) THEN 'accepted'
				ELSE 'available'
			END AS challenge_status
		FROM photos p
		WHERE p.id = ANY($1)`, photoIDs, viewerID)
	if err != nil {
		return MessagesPage{}, err
	}
	defer rows.Close()
	statuses := make(map[string]string, len(photoIDs))
	for rows.Next() {
		var photoID, status string
		if err := rows.Scan(&photoID, &status); err != nil {
			return MessagesPage{}, err
		}
		statuses[photoID] = status
	}
	if err := rows.Err(); err != nil {
		return MessagesPage{}, err
	}
	for index := range page.Items {
		if page.Items[index].PhotoID != nil {
			page.Items[index].ChallengeStatus = statuses[*page.Items[index].PhotoID]
		}
	}
	return page, nil
}

// scanMessageRows drains a message result set (closing it) into a slice in the
// order the database returned it.
func scanMessageRows(rows pgx.Rows) ([]models.Message, error) {
	defer rows.Close()
	messages := make([]models.Message, 0)
	for rows.Next() {
		var msg models.Message
		var username, avatar sql.NullString
		if err := rows.Scan(&msg.ID, &msg.GroupID, &msg.UserID, &username, &avatar, &msg.Kind, &msg.PhotoID, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
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
		return nil, err
	}
	return messages, nil
}

// reverseMessages reverses the slice in place.
func reverseMessages(messages []models.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// CursorAfterMessage resolves a legacy message id into the opaque cursor that
// positions pagination immediately after it. An empty or unknown message id
// yields an empty cursor so the caller falls back to the latest page instead
// of failing the whole request.
func CursorAfterMessage(ctx context.Context, messageID string) (string, error) {
	if messageID == "" {
		return "", nil
	}
	var createdAt time.Time
	err := database.DB.QueryRow(ctx, `SELECT created_at FROM messages WHERE id = $1`, messageID).Scan(&createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return encodeMessageCursor(createdAt, messageID), nil
}

// GetGroupMessagesContext remains as a thin cursor wrapper for any caller that
// still passes a legacy message id. New callers should use the page API.
func GetGroupMessagesContext(ctx context.Context, groupID, afterID string) ([]models.Message, error) {
	cursor, err := CursorAfterMessage(ctx, afterID)
	if err != nil {
		return nil, err
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
