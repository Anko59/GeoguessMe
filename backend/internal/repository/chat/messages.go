package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// resolveSenderProfile fills the message's username and avatar from the users
// table when they are not already present. It is the single canonical sender
// lookup for both text-message and chat-media-message creation: the lookup is
// best-effort (a missing profile never fails the write).
func (r *Repository) resolveSenderProfile(ctx context.Context, q rowQuerier, msg *models.Message) {
	if msg.Username != "" {
		return
	}
	var username, avatar string
	if err := q.QueryRow(ctx, `SELECT username, avatar FROM users WHERE id = $1`, msg.UserID).Scan(&username, &avatar); err == nil {
		msg.Username, msg.Avatar = username, avatar
	}
}

// validateReplyTarget rejects a reply_to_id that does not reference an existing
// message in the same group. It is the single canonical reply validation for
// every message creation path.
func (r *Repository) validateReplyTarget(ctx context.Context, q rowQuerier, groupID string, replyToID *string) error {
	if replyToID == nil {
		return nil
	}
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE id = $1 AND group_id = $2)`, *replyToID, groupID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrInvalidMessageReply
	}
	return nil
}

// SaveMessage persists a text message, resolving the sender profile and
// validating the reply target through the shared canonical helpers.
func (r *Repository) SaveMessage(ctx context.Context, msg *models.Message) error {
	r.resolveSenderProfile(ctx, r.pool, msg)
	if err := r.validateReplyTarget(ctx, r.pool, msg.GroupID, msg.ReplyToID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO messages(id, group_id, user_id, kind, photo_id, reply_to_id, content, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, msg.ID, msg.GroupID, msg.UserID, msg.Kind, msg.PhotoID, msg.ReplyToID, msg.Content, msg.CreatedAt)
	return err
}

// GetGroupMessagesPage returns a page of group messages.
//
// An empty cursor selects the most recent page: the newest `limit` messages,
// returned in chronological (ascending) order, with an empty next_cursor
// because no newer pages exist. A non-empty opaque cursor returns the messages
// strictly after that cursor in ascending order; next_cursor is set when more
// pages remain and empty otherwise. Ordering always follows the stable tuple
// (created_at, id) so reconnect catch-up cannot skip or duplicate a message.
//
// StableCursor is populated for every non-empty page and points strictly after
// the last message of that page, so a client can snapshot it and resume
// catch-up losslessly after a reconnect.
func (r *Repository) GetGroupMessagesPage(ctx context.Context, groupID, cursor string, limit int) (MessagesPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	if cursor == "" {
		query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id LEFT JOIN chat_media cm ON m.media_id = cm.id WHERE m.group_id = $1 ORDER BY m.created_at DESC, m.id DESC LIMIT $2`
		rows, err := r.pool.Query(ctx, query, groupID, limit)
		if err != nil {
			return MessagesPage{}, err
		}
		messages, err := scanMessageRows(rows)
		if err != nil {
			return MessagesPage{}, err
		}
		// Fetch newest-first but expose the page in chronological order.
		reverseMessages(messages)
		page := MessagesPage{Items: messages}
		if len(messages) > 0 {
			last := messages[len(messages)-1]
			page.StableCursor = encodeMessageCursor(last.CreatedAt, last.ID)
		}
		return page, nil
	}

	createdAt, id, err := decodeMessageCursor(cursor)
	if err != nil {
		return MessagesPage{}, fmt.Errorf("invalid message cursor: %w", err)
	}
	query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id LEFT JOIN chat_media cm ON m.media_id = cm.id WHERE m.group_id = $1 AND ROW(m.created_at, m.id) > ROW($2, $3) ORDER BY m.created_at ASC, m.id ASC LIMIT $4`
	rows, err := r.pool.Query(ctx, query, groupID, createdAt, id, limit+1)
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
	if len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		page.StableCursor = encodeMessageCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// GetGroupMessagesPageBefore returns up to limit messages strictly before the
// message identified by beforeID, in chronological (ascending) order, with an
// empty NextCursor. The caller derives the next older request from the oldest
// returned message. An unknown or out-of-group beforeID yields an empty page
// (there is nothing older to load).
func (r *Repository) GetGroupMessagesPageBefore(ctx context.Context, groupID, beforeID string, limit int) (MessagesPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT created_at FROM messages WHERE id = $1 AND group_id = $2`, beforeID, groupID).Scan(&createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessagesPage{Items: []models.Message{}}, nil
		}
		return MessagesPage{}, err
	}
	query := `SELECT ` + messageColumns + ` FROM messages m LEFT JOIN users u ON m.user_id = u.id LEFT JOIN chat_media cm ON m.media_id = cm.id WHERE m.group_id = $1 AND ROW(m.created_at, m.id) < ROW($2, $3) ORDER BY m.created_at DESC, m.id DESC LIMIT $4`
	rows, err := r.pool.Query(ctx, query, groupID, createdAt, beforeID, limit)
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

// GetGroupMessagesPageForViewer enriches challenge messages with state that is
// specific to the authenticated viewer. The state is derived from the
// existing challenge views and guesses tables, so reconnects and hard reloads
// restore the same action shown in the chat without client-only assumptions.
func (r *Repository) GetGroupMessagesPageForViewer(ctx context.Context, groupID, cursor string, limit int, viewerID string) (MessagesPage, error) {
	page, err := r.GetGroupMessagesPage(ctx, groupID, cursor, limit)
	if err != nil {
		return page, err
	}
	return r.EnrichMessagesPageForViewer(ctx, page, viewerID)
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
