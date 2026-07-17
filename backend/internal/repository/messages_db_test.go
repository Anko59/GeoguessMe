package repository

import (
	"context"
	"testing"
	"time"

	"geoguessme/internal/models"
	"github.com/pashagolub/pgxmock/v4"
)

func messageRows(times ...time.Time) *pgxmock.Rows {
	rows := pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "content", "created_at"})
	for i, createdAt := range times {
		rows.AddRow("message-"+string(rune('a'+i)), "group-1", "user-1", "alice", "avatar.png", "text", nil, "hello", createdAt)
	}
	return rows
}

func TestMessagePersistenceAndPagination(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	var photoID *string
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"))
	mock.ExpectExec("INSERT INTO messages").WithArgs("message-1", "group-1", "user-1", "text", photoID, "hello", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	message := &models.Message{ID: "message-1", GroupID: "group-1", UserID: "user-1", Kind: "text", Content: "hello", CreatedAt: now}
	if err := SaveMessageContext(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if message.Username != "alice" || message.Avatar != "avatar.png" {
		t.Fatalf("message profile = %+v", message)
	}

	times := []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}
	mock.ExpectQuery("SELECT .*FROM messages").WithArgs("group-1", 3).WillReturnRows(messageRows(times...))
	page, err := GetGroupMessagesPage(context.Background(), "group-1", "", 2)
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	createdAt, id, err := decodeMessageCursor(page.NextCursor)
	if err != nil || !createdAt.Equal(times[1]) || id != "message-b" {
		t.Fatalf("next cursor = %v/%q, %v", createdAt, id, err)
	}
	mock.ExpectQuery("SELECT .*FROM messages").WithArgs("group-1", times[1], "message-b", 3).WillReturnRows(messageRows(times[2]))
	page, err = GetGroupMessagesPage(context.Background(), "group-1", page.NextCursor, 2)
	if err != nil || len(page.Items) != 1 || page.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", page, err)
	}
	if _, err := GetGroupMessagesPage(context.Background(), "group-1", "not-a-cursor", 2); err == nil {
		t.Fatal("malformed cursor accepted")
	}

	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
	mock.ExpectQuery("SELECT .*FROM messages").WithArgs("group-1", now, "message-1", 501).WillReturnRows(messageRows(times[1]))
	messages, err := GetGroupMessagesContext(context.Background(), "group-1", "message-1")
	if err != nil || len(messages) != 1 {
		t.Fatalf("legacy messages = %+v, %v", messages, err)
	}
	newMessage := NewTextMessage("group-1", "user-1", "content", now)
	if newMessage.Kind != "text" || newMessage.ID == "" || !newMessage.CreatedAt.Equal(now) {
		t.Fatalf("new text message = %+v", newMessage)
	}
}
