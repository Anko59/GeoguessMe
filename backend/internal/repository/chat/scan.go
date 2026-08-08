package chat

import (
	"context"
	"database/sql"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// messageColumns is the SELECT projection shared by every message read. This
// file is the single owner of the message row shape: scans and pagination
// must not redefine the columns or reorder them.
const messageColumns = "m.id, m.group_id, m.user_id, u.username, u.avatar, m.kind, m.photo_id, m.media_id, cm.mime_type, m.reply_to_id, m.content, m.created_at"

// rowQuerier is satisfied by both the injected pool and a pgx transaction, so
// the shared sender-resolution and reply-validation helpers can run inside
// chat media transactions as well as on the top-level pool.
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// scanMessageRows drains a message result set (closing it) into a slice in the
// order the database returned it.
func scanMessageRows(rows pgx.Rows) ([]models.Message, error) {
	defer rows.Close()
	messages := make([]models.Message, 0)
	for rows.Next() {
		var msg models.Message
		var username, avatar sql.NullString
		var mediaID, mediaType, replyToID sql.NullString
		if err := rows.Scan(&msg.ID, &msg.GroupID, &msg.UserID, &username, &avatar, &msg.Kind, &msg.PhotoID, &mediaID, &mediaType, &replyToID, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if username.Valid {
			msg.Username = username.String
		}
		if avatar.Valid {
			msg.Avatar = avatar.String
		}
		if replyToID.Valid {
			msg.ReplyToID = &replyToID.String
		}
		if mediaID.Valid {
			msg.MediaID = &mediaID.String
		}
		if mediaType.Valid {
			msg.MediaType = mediaType.String
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
