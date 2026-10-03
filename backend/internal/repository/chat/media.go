package chat

import (
	"context"
	"errors"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// CreateChatMediaMessage atomically records an uploaded private attachment and
// its message. The caller stores the object first and compensates that object
// if this transaction fails, matching the challenge-upload durability model.
// Sender resolution and reply validation reuse the same canonical helpers as
// text-message creation (SaveMessage), so the two paths cannot drift.
func (r *Repository) CreateChatMediaMessage(ctx context.Context, msg *models.Message, asset *models.ChatMedia) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.CreateChatMediaMessageTx(ctx, tx, msg, asset); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateChatMediaMessageTx records the attachment and its message using the
// supplied transaction. It is the tx-scoped variant used by the
// media-processing worker so the chat message and the job completion commit
// atomically. On success it fills msg with the media reference, matching
// CreateChatMediaMessage.
func (r *Repository) CreateChatMediaMessageTx(ctx context.Context, tx pgx.Tx, msg *models.Message, asset *models.ChatMedia) error {
	if msg == nil || asset == nil || msg.ID == "" || asset.ID == "" || msg.GroupID != asset.GroupID || msg.UserID != asset.UserID {
		return errors.New("invalid chat media message")
	}
	r.resolveSenderProfile(ctx, tx, msg)
	if err := r.validateReplyTarget(ctx, tx, msg.GroupID, msg.UserID, msg.ReplyToID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chat_media(id, group_id, user_id, storage_key, mime_type, byte_size, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, asset.ID, asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO messages(id, group_id, user_id, kind, media_id, reply_to_id, content, created_at)
		SELECT $1,$2,$3,'media',$4,$5,$6,$7
		WHERE EXISTS (SELECT 1 FROM group_members WHERE group_id=$2 AND user_id=$3)
		AND ($5::text IS NULL OR EXISTS (SELECT 1 FROM messages m WHERE m.id=$5 AND m.group_id=$2 AND `+messageVisibility("$3")+`))`, msg.ID, msg.GroupID, msg.UserID, asset.ID, msg.ReplyToID, msg.Content, msg.CreatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMessageForbidden
	}
	msg.Kind = "media"
	msg.MediaID = &asset.ID
	msg.MediaType = asset.MIMEType
	return nil
}

// GetChatMedia returns only an attachment already referenced by a message.
// Unattached storage records are never readable, including by their uploader.
func (r *Repository) GetChatMedia(ctx context.Context, mediaID string) (*models.ChatMedia, error) {
	return r.getChatMedia(ctx, mediaID)
}

func (r *Repository) getChatMedia(ctx context.Context, mediaID string, viewers ...string) (*models.ChatMedia, error) {
	asset := &models.ChatMedia{ID: mediaID}
	query, args := viewerMessageQuery(`SELECT cm.group_id, cm.user_id, cm.storage_key, cm.mime_type, cm.byte_size, cm.created_at FROM chat_media cm JOIN messages m ON m.media_id = cm.id WHERE cm.id = $1`, []any{mediaID}, viewers...)
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&asset.GroupID, &asset.UserID, &asset.StorageKey, &asset.MIMEType, &asset.ByteSize, &asset.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return asset, nil
}
