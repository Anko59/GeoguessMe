package chat

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestCreateAndConsumeWebSocketTicket(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	mock.ExpectExec("INSERT INTO websocket_tickets").
		WithArgs("ticket-1", "user-1", "group-1", "hash", now.Add(time.Minute)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateWebSocketTicket(ctx, "ticket-1", "user-1", "group-1", "hash", now.Add(time.Minute)); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	// A valid ticket is atomically consumed and returns the owning user.
	mock.ExpectQuery("UPDATE websocket_tickets SET used_at").
		WithArgs("hash", "group-1").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
	userID, err := repo.ConsumeWebSocketTicket(ctx, "hash", "group-1")
	if err != nil || userID != "user-1" {
		t.Fatalf("consume ticket = %q, %v", userID, err)
	}

	// A consumed, missing, or expired ticket yields an empty user id instead of
	// an error so the WebSocket upgrade can reject it cleanly.
	mock.ExpectQuery("UPDATE websocket_tickets SET used_at").
		WithArgs("hash", "group-1").
		WillReturnError(pgx.ErrNoRows)
	userID, err = repo.ConsumeWebSocketTicket(ctx, "hash", "group-1")
	if err != nil || userID != "" {
		t.Fatalf("reuse ticket = %q, %v (want empty user, no error)", userID, err)
	}
}
