package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

func TestViewerMessageStateAndReactions(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	photoID := "photo-1"

	mock.ExpectQuery("SELECT .*FROM messages.*ORDER BY m.created_at DESC").
		WithArgs("group-1", 10).
		WillReturnRows(singleMessageRow("message-1", "challenge", &photoID, now))
	mock.ExpectQuery("SELECT message_id, reaction, COUNT").
		WithArgs([]string{"message-1"}, "viewer-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}).
			AddRow("message-1", "like", 2, true, []string{"alice", "bob"}))
	mock.ExpectQuery("SELECT p.id,").
		WithArgs([]string{photoID}, "viewer-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "expires_at", "ttl_seconds", "challenge_status", "challenge_resolved"}).
			AddRow(photoID, now.Add(time.Hour), int64(86400), "guessed", true))
	page, err := repo.GetGroupMessagesPageForViewer(ctx, "group-1", "", 10, "viewer-1")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("viewer message page = %+v, %v", page, err)
	}
	message := page.Items[0]
	if message.ChallengeStatus != "guessed" || !message.ChallengeResolved ||
		message.ChallengeExpiresAt == nil || !message.ChallengeExpiresAt.After(now) || message.ChallengeTTLSeconds != 86400 ||
		len(message.Reactions) != 1 || !message.Reactions[0].Reacted || message.Reactions[0].Count != 2 ||
		len(message.Reactions[0].Usernames) != 2 || message.Reactions[0].Usernames[1] != "bob" {
		t.Fatalf("enriched message = %+v", message)
	}

	mock.ExpectQuery("SELECT .*FROM messages.*WHERE m.id").
		WithArgs("message-1").
		WillReturnRows(singleMessageRow("message-1", "text", nil, now))
	mock.ExpectQuery("SELECT message_id, reaction, COUNT").
		WithArgs([]string{"message-1"}, "viewer-2").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
	loaded, err := repo.GetMessageForViewer(ctx, "message-1", "viewer-2")
	if err != nil || loaded == nil || loaded.Reactions == nil {
		t.Fatalf("message for viewer = %+v, %v", loaded, err)
	}

	mock.ExpectQuery("SELECT .*FROM messages.*m.photo_id").
		WithArgs(photoID).
		WillReturnRows(singleMessageRow("challenge-message", "challenge", &photoID, now))
	mock.ExpectQuery("SELECT message_id, reaction, COUNT").
		WithArgs([]string{"challenge-message"}, "").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
	loaded, err = repo.GetChallengeMessageForViewer(ctx, photoID, "")
	if err != nil || loaded == nil || loaded.ID != "challenge-message" {
		t.Fatalf("challenge message = %+v, %v", loaded, err)
	}
}

func TestMessagePersistenceAndPagination(t *testing.T) {
	repo, mock := newChatRepo(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	var photoID *string
	var replyToID *string
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"))
	mock.ExpectExec("INSERT INTO messages").WithArgs("message-1", "group-1", "user-1", "text", photoID, replyToID, "hello", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	message := &models.Message{ID: "message-1", GroupID: "group-1", UserID: "user-1", Kind: "text", Content: "hello", CreatedAt: now}
	if err := repo.SaveMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if message.Username != "alice" || message.Avatar != "avatar.png" {
		t.Fatalf("message profile = %+v", message)
	}
	replyID := "message-parent"
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(replyID, "group-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO messages").WithArgs("message-2", "group-1", "user-1", "text", photoID, &replyID, "reply", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.SaveMessage(context.Background(), &models.Message{ID: "message-2", GroupID: "group-1", UserID: "user-1", Kind: "text", ReplyToID: &replyID, Content: "reply", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	missing := "missing"
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar.png"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(missing, "group-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if err := repo.SaveMessage(context.Background(), &models.Message{ID: "message-3", GroupID: "group-1", UserID: "user-1", Kind: "text", ReplyToID: &missing, Content: "reply", CreatedAt: now}); !errors.Is(err, ErrInvalidMessageReply) {
		t.Fatalf("invalid reply = %v", err)
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
	page, err := repo.GetGroupMessagesPage(context.Background(), "group-1", "", 2)
	if err != nil {
		t.Fatalf("latest page: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("latest page = %+v, %v", page, err)
	}
	if page.Items[0].ID != "message-c" || page.Items[1].ID != "message-d" {
		t.Fatalf("latest page order = %s, %s, want message-c, message-d", page.Items[0].ID, page.Items[1].ID)
	}
	if page.StableCursor != encodeMessageCursor(t3, "message-d") {
		t.Fatalf("latest page stable cursor = %q", page.StableCursor)
	}

	// Forward catch-up from the oldest cursor returns the newer messages and
	// reports a next cursor when more remain (limit+1 probes for another page).
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) > ROW").
		WithArgs("group-1", t0, "message-a", 3).
		WillReturnRows(messageRowsByID([]string{"message-b", "message-c", "message-d"}, []time.Time{t1, t2, t3}))
	page, err = repo.GetGroupMessagesPage(context.Background(), "group-1", encodeMessageCursor(t0, "message-a"), 2)
	if err != nil {
		t.Fatalf("forward page: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("forward page = %+v, %v", page, err)
	}
	if page.Items[0].ID != "message-b" || page.Items[1].ID != "message-c" {
		t.Fatalf("forward page order = %s, %s, want message-b, message-c", page.Items[0].ID, page.Items[1].ID)
	}
	if page.StableCursor != encodeMessageCursor(t2, "message-c") {
		t.Fatalf("forward page stable cursor = %q", page.StableCursor)
	}
	cursorAt, cursorID, err := decodeMessageCursor(page.NextCursor)
	if err != nil || !cursorAt.Equal(t2) || cursorID != "message-c" {
		t.Fatalf("next cursor = %v/%q, %v", cursorAt, cursorID, err)
	}

	// The final forward page drains the remaining message and clears the cursor.
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) > ROW").
		WithArgs("group-1", t2, "message-c", 3).
		WillReturnRows(messageRowsByID([]string{"message-d"}, []time.Time{t3}))
	page, err = repo.GetGroupMessagesPage(context.Background(), "group-1", page.NextCursor, 2)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "message-d" || page.NextCursor != "" {
		t.Fatalf("final page = %+v, %v", page, err)
	}
	if page.StableCursor != encodeMessageCursor(t3, "message-d") {
		t.Fatalf("final page stable cursor = %q", page.StableCursor)
	}

	// A malformed cursor must be rejected rather than silently returning data.
	if _, err := repo.GetGroupMessagesPage(context.Background(), "group-1", "not-a-cursor", 2); err == nil {
		t.Fatal("malformed cursor accepted")
	}

	newMessage := NewTextMessage("group-1", "user-1", "content", now)
	if newMessage.Kind != "text" || newMessage.ID == "" || !newMessage.CreatedAt.Equal(now) {
		t.Fatalf("new text message = %+v", newMessage)
	}
}

// TestGetGroupMessagesPageBefore covers the backward pagination direction used
// to prepend chat history when the client scrolls up: the page is strictly
// before the referenced message, chronological, capped at limit, and drains to
// an empty page at the start of the conversation.
func TestGetGroupMessagesPageBefore(t *testing.T) {
	repo, mock := newChatRepo(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	t0 := now
	t1 := now.Add(time.Second)
	t2 := now.Add(2 * time.Second)
	t3 := now.Add(3 * time.Second)

	// Resolve the anchor position (scoped to the group) then fetch the older
	// page newest-first (DESC) and expose it chronologically.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-d", "group-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(t3))
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) < ROW").
		WithArgs("group-1", t3, "message-d", 2).
		WillReturnRows(messageRowsByID([]string{"message-c", "message-b"}, []time.Time{t2, t1}))
	page, err := repo.GetGroupMessagesPageBefore(context.Background(), "group-1", "message-d", 2)
	if err != nil {
		t.Fatalf("older page: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("older page = %+v", page)
	}
	if page.Items[0].ID != "message-b" || page.Items[1].ID != "message-c" {
		t.Fatalf("older page order = %s, %s, want message-b, message-c", page.Items[0].ID, page.Items[1].ID)
	}

	// A page that starts at the conversation start returns fewer than limit
	// items so the client knows history is exhausted.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-b", "group-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}).AddRow(t1))
	mock.ExpectQuery("SELECT .*FROM messages.*ROW\\(m.created_at, m.id\\) < ROW").
		WithArgs("group-1", t1, "message-b", 2).
		WillReturnRows(messageRowsByID([]string{"message-a"}, []time.Time{t0}))
	page, err = repo.GetGroupMessagesPageBefore(context.Background(), "group-1", "message-b", 2)
	if err != nil {
		t.Fatalf("draining page: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "message-a" {
		t.Fatalf("draining page = %+v", page)
	}

	// An unknown anchor (or one outside the group) yields an empty page instead
	// of failing, so a stale client cursor never errors out.
	mock.ExpectQuery("SELECT created_at FROM messages").WithArgs("message-x", "group-1").WillReturnRows(pgxmock.NewRows([]string{"created_at"}))
	page, err = repo.GetGroupMessagesPageBefore(context.Background(), "group-1", "message-x", 2)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("unknown anchor = %+v, %v", page, err)
	}
}
