package chat

import (
	"context"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestChatMediaMessagePersistenceAndLookup(t *testing.T) {
	repo, mock := newChatRepo(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	message := &models.Message{ID: "message-1", GroupID: "group-1", UserID: "user-1", Content: "look at this", CreatedAt: now}
	asset := &models.ChatMedia{ID: "media-1", GroupID: "group-1", UserID: "user-1", StorageKey: "chat-media/object", MIMEType: "image/png", ByteSize: 42, CreatedAt: now}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(
		pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"),
	)
	mock.ExpectExec("INSERT INTO chat_media").WithArgs(asset.ID, asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO messages").WithArgs(message.ID, message.GroupID, message.UserID, asset.ID, message.ReplyToID, message.Content, message.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := repo.CreateChatMediaMessage(context.Background(), message, asset); err != nil {
		t.Fatalf("create chat media message: %v", err)
	}
	if message.Kind != "media" || message.MediaID == nil || *message.MediaID != asset.ID || message.MediaType != asset.MIMEType {
		t.Fatalf("persisted message = %+v", message)
	}
	if message.Username != "alice" || message.Avatar != "avatar.png" {
		t.Fatalf("message sender = %+v", message)
	}
	replyID := "parent-message"
	replyAsset := &models.ChatMedia{ID: "media-2", GroupID: "group-1", UserID: "user-1", StorageKey: "chat-media/reply", MIMEType: "video/webm", ByteSize: 24, CreatedAt: now}
	replyMessage := &models.Message{ID: "message-2", GroupID: "group-1", UserID: "user-1", Username: "alice", Avatar: "avatar.png", ReplyToID: &replyID, Content: "reply with a video", CreatedAt: now}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(replyID, replyMessage.GroupID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO chat_media").WithArgs(replyAsset.ID, replyAsset.GroupID, replyAsset.UserID, replyAsset.StorageKey, replyAsset.MIMEType, replyAsset.ByteSize, replyAsset.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO messages").WithArgs(replyMessage.ID, replyMessage.GroupID, replyMessage.UserID, replyAsset.ID, replyMessage.ReplyToID, replyMessage.Content, replyMessage.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := repo.CreateChatMediaMessage(context.Background(), replyMessage, replyAsset); err != nil {
		t.Fatalf("create reply chat media message: %v", err)
	}

	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs(asset.ID).WillReturnRows(
		pgxmock.NewRows([]string{"group_id", "user_id", "storage_key", "mime_type", "byte_size", "created_at"}).AddRow(asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt),
	)
	loaded, err := repo.GetChatMedia(context.Background(), asset.ID)
	if err != nil || loaded == nil {
		t.Fatalf("get chat media = %+v, %v", loaded, err)
	}
	if *loaded != *asset {
		t.Fatalf("loaded asset = %+v, want %+v", loaded, asset)
	}

	mock.ExpectQuery("SELECT cm.group_id, cm.user_id, cm.storage_key").WithArgs("unattached").WillReturnError(pgx.ErrNoRows)
	missing, err := repo.GetChatMedia(context.Background(), "unattached")
	if err != nil || missing != nil {
		t.Fatalf("unattached media = %+v, %v", missing, err)
	}

	invalid := []*models.Message{nil, {ID: "", GroupID: "group-1", UserID: "user-1"}, {ID: "message", GroupID: "other", UserID: "user-1"}}
	for _, candidate := range invalid {
		if err := repo.CreateChatMediaMessage(context.Background(), candidate, asset); err == nil {
			t.Fatalf("invalid message %#v was accepted", candidate)
		}
	}
	if err := repo.CreateChatMediaMessage(context.Background(), message, nil); err == nil {
		t.Fatal("nil asset was accepted")
	}
}
