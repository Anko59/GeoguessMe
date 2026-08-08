package chat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

// newMockPool returns an isolated mock pool bound directly to a chat
// Repository. Unlike the old package-level tests it never swaps the
// database.DB global: every chat query goes through the injected pool.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

func newChatRepo(t *testing.T) (*Repository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock := newMockPool(t)
	return NewRepository(mock), mock
}

// messageRowsByID builds message rows pairing each id with its created_at in
// the order given, so pagination assertions read in the exact order the
// database would return them.
func messageRowsByID(ids []string, times []time.Time) *pgxmock.Rows {
	rows := pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"})
	for i, id := range ids {
		rows.AddRow(id, "group-1", "user-1", "alice", "avatar.png", "text", nil, nil, nil, nil, "hello", times[i])
	}
	return rows
}

func singleMessageRow(id, kind string, photoID *string, createdAt time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "avatar", "kind", "photo_id", "media_id", "mime_type", "reply_to_id", "content", "created_at"}).
		AddRow(id, "group-1", "user-1", "alice", "avatar.png", kind, photoID, nil, nil, nil, "hello", createdAt)
}

func TestMessageCursorRoundTrip(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 123456789, time.UTC)
	const id = "f1e2d3c4-0000-0000-0000-000000000001"
	cursor := encodeMessageCursor(now, id)
	gotAt, gotID, err := decodeMessageCursor(cursor)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotID != id || !gotAt.Equal(now) {
		t.Fatalf("round trip mismatch: got %s @ %v want %s @ %v", gotID, gotAt, id, now)
	}
}

func TestMessageCursorRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!", "++++", "onlyonepart", "abc|"} {
		if _, _, err := decodeMessageCursor(bad); err == nil {
			t.Errorf("expected error for cursor %q", bad)
		}
	}
}

// enrichBenchPage builds a page of n challenge messages, each with its own
// photo id, ready for EnrichMessagesPageForViewer.
func enrichBenchPage(n int) (MessagesPage, []string) {
	page := MessagesPage{Items: make([]models.Message, 0, n)}
	photoIDs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		photoID := fmt.Sprintf("photo-%04d", i)
		photoIDs = append(photoIDs, photoID)
		page.Items = append(page.Items, models.Message{ID: "message-" + photoID, Kind: "challenge", PhotoID: &photoIDs[i], CreatedAt: time.Unix(int64(i), 0).UTC()})
	}
	return page, photoIDs
}

// TestEnrichMessagesPageForViewerIsLinear pins the enrichment contract: the
// photo-state query returns one row per photo and every row lands on the
// indexed message. The mock expects exactly two queries regardless of page
// size, so a page that re-queried per message would fail the mock (an
// unchecked expectation). BenchmarkEnrichMessagesPageForViewer pins the
// runtime scaling.
func TestEnrichMessagesPageForViewerIsLinear(t *testing.T) {
	const pageSize = 200
	repo, mock := newChatRepo(t)
	page, photoIDs := enrichBenchPage(pageSize)
	messageIDs := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		messageIDs = append(messageIDs, item.ID)
	}
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT message_id, reaction, COUNT").
		WithArgs(messageIDs, "viewer-1").
		WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
	statusRows := pgxmock.NewRows([]string{"id", "expires_at", "ttl_seconds", "challenge_status", "challenge_resolved"})
	for _, photoID := range photoIDs {
		statusRows.AddRow(photoID, now.Add(time.Hour), int64(86400), "available", false)
	}
	mock.ExpectQuery("SELECT p.id,").WithArgs(photoIDs, "viewer-1").WillReturnRows(statusRows)

	enriched, err := repo.EnrichMessagesPageForViewer(context.Background(), page, "viewer-1")
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(enriched.Items) != pageSize {
		t.Fatalf("enriched page size = %d", len(enriched.Items))
	}
	for index, item := range enriched.Items {
		if item.ChallengeStatus != "available" || item.ChallengeExpiresAt == nil || item.ChallengeTTLSeconds != 86400 {
			t.Fatalf("item %d not enriched: %+v", index, item)
		}
	}
}

// BenchmarkEnrichMessagesPageForViewer demonstrates linear enrichment: the
// per-iteration cost should grow roughly proportionally to the page size
// (messages plus linked photos), not quadratically. Run with
// `go test ./internal/repository/chat -bench=EnrichMessagesPageForViewer -benchmem`.
func BenchmarkEnrichMessagesPageForViewer(b *testing.B) {
	for _, size := range []int{50, 200, 800} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			page, photoIDs := enrichBenchPage(size)
			messageIDs := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				messageIDs = append(messageIDs, item.ID)
			}
			now := time.Now().UTC()
			statusRows := pgxmock.NewRows([]string{"id", "expires_at", "ttl_seconds", "challenge_status", "challenge_resolved"})
			for _, photoID := range photoIDs {
				statusRows.AddRow(photoID, now.Add(time.Hour), int64(86400), "available", false)
			}
			mock, err := pgxmock.NewPool()
			if err != nil {
				b.Fatal(err)
			}
			repo := NewRepository(mock)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Each iteration queues its own expectations in FIFO order;
				// the repository still issues exactly two queries per page.
				mock.ExpectQuery("SELECT message_id, reaction, COUNT").
					WithArgs(messageIDs, "viewer-1").
					WillReturnRows(pgxmock.NewRows([]string{"message_id", "reaction", "count", "reacted", "usernames"}))
				mock.ExpectQuery("SELECT p.id,").WithArgs(photoIDs, "viewer-1").WillReturnRows(statusRows)
				if _, err := repo.EnrichMessagesPageForViewer(context.Background(), page, "viewer-1"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
