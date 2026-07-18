package repository

import (
	"context"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

// messageRowsByID builds message rows pairing each id with its created_at in
// the order given, so pagination assertions read in the exact order the
// database would return them.
func messageRowsByID(ids []string, times []time.Time) *pgxmock.Rows {
	rows := pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "content", "created_at"})
	for i, id := range ids {
		rows.AddRow(id, "group-1", "user-1", "alice", "avatar.png", "text", nil, "hello", times[i])
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

	// Four messages ordered oldest -> newest.
	t0 := now
	t1 := now.Add(time.Second)
	t2 := now.Add(2 * time.Second)
	t3 := now.Add(3 * time.Second)

	// An empty cursor selects the latest page: the database returns the newest
	// messages first (DESC) and the repository exposes them chronologically
	// with no forward cursor because nothing newer exists.
	mock.ExpectQuery("SELECT .*FROM messages.*ORDER BY m.created_at DESC").
		WithArgs("group-1", 2).
		WillReturnRows(messageRowsByID([]string{"message-d", "message-c"}, []time.Time{t3, t2}))
	page, err := GetGroupMessagesPage(context.Background(), "group-1", "", 2)
	if err != nil {
		t.Fatalf("latest page: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("latest page = %+v, %v", page, err)
	}
	if page.Items[0].ID != "message-c" || page.Items[1].ID != "message-d" {
		t.Fatalf("latest page order = %s, %s, want message-c, message-d", page.Items[0].ID, page.Items[1].ID)
	}

	// Forward catch-up from the oldest cursor returns the newer messages and
	// reports a next cursor when more remain (limit+1 probes for another page).
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) > ROW").
		WithArgs("group-1", t0, "message-a", 3).
		WillReturnRows(messageRowsByID([]string{"message-b", "message-c", "message-d"}, []time.Time{t1, t2, t3}))
	page, err = GetGroupMessagesPage(context.Background(), "group-1", encodeMessageCursor(t0, "message-a"), 2)
	if err != nil {
		t.Fatalf("forward page: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("forward page = %+v, %v", page, err)
	}
	if page.Items[0].ID != "message-b" || page.Items[1].ID != "message-c" {
		t.Fatalf("forward page order = %s, %s, want message-b, message-c", page.Items[0].ID, page.Items[1].ID)
	}
	cursorAt, cursorID, err := decodeMessageCursor(page.NextCursor)
	if err != nil || !cursorAt.Equal(t2) || cursorID != "message-c" {
		t.Fatalf("next cursor = %v/%q, %v", cursorAt, cursorID, err)
	}

	// The final forward page drains the remaining message and clears the cursor.
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) > ROW").
		WithArgs("group-1", t2, "message-c", 3).
		WillReturnRows(messageRowsByID([]string{"message-d"}, []time.Time{t3}))
	page, err = GetGroupMessagesPage(context.Background(), "group-1", page.NextCursor, 2)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "message-d" || page.NextCursor != "" {
		t.Fatalf("final page = %+v, %v", page, err)
	}

	// A malformed cursor must be rejected rather than silently returning data.
	if _, err := GetGroupMessagesPage(context.Background(), "group-1", "not-a-cursor", 2); err == nil {
		t.Fatal("malformed cursor accepted")
	}

	// The legacy after_id wrapper resolves the message id to the opaque cursor
	// and then paginates forward through the remaining messages.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-a").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(t0))
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) > ROW").
		WithArgs("group-1", t0, "message-a", 501).
		WillReturnRows(messageRowsByID([]string{"message-b", "message-c", "message-d"}, []time.Time{t1, t2, t3}))
	messages, err := GetGroupMessagesContext(context.Background(), "group-1", "message-a")
	if err != nil {
		t.Fatalf("legacy messages: %v", err)
	}
	if len(messages) != 3 || messages[0].ID != "message-b" || messages[2].ID != "message-d" {
		t.Fatalf("legacy messages = %+v", messages)
	}

	newMessage := NewTextMessage("group-1", "user-1", "content", now)
	if newMessage.Kind != "text" || newMessage.ID == "" || !newMessage.CreatedAt.Equal(now) {
		t.Fatalf("new text message = %+v", newMessage)
	}
}

// TestCursorAfterMessageResolvesLegacyID covers the bridge from the legacy
// after_id message id onto the stable opaque cursor, including the empty and
// unknown-id fallbacks that keep a reconnect catch-up request from failing.
func TestCursorAfterMessageResolvesLegacyID(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// An empty after id short-circuits to an empty cursor without querying.
	got, err := CursorAfterMessage(context.Background(), "")
	if err != nil || got != "" {
		t.Fatalf("empty after id = %q, %v", got, err)
	}

	// A known message id resolves to the opaque cursor at its position.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(now))
	got, err = CursorAfterMessage(context.Background(), "message-1")
	if err != nil {
		t.Fatalf("resolve known id: %v", err)
	}
	createdAt, id, err := decodeMessageCursor(got)
	if err != nil || !createdAt.Equal(now) || id != "message-1" {
		t.Fatalf("resolved cursor = %v/%q, %v", createdAt, id, err)
	}

	// An unknown message id yields an empty cursor (latest-page fallback)
	// instead of failing the whole pagination request.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	got, err = CursorAfterMessage(context.Background(), "missing")
	if err != nil || got != "" {
		t.Fatalf("unknown after id = %q, %v", got, err)
	}
}
