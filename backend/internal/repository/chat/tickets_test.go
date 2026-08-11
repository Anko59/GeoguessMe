package chat

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

// consumeSQL pins the security-relevant shape of the atomic consume UPDATE:
// a ticket is marked used only when it is still unused and unexpired, the
// owning user is active (deleted_at IS NULL), the ticket's auth_version still
// equals the user's current version, and a live group_members row still
// exists. Any regression that drops one of these predicates must fail this
// test.
const consumeSQL = `UPDATE websocket_tickets t SET used_at.*FROM users u.*used_at IS NULL.*expires_at > CURRENT_TIMESTAMP.*deleted_at IS NULL.*auth_version = u\.auth_version.*EXISTS \(SELECT 1 FROM group_members gm.*RETURNING t\.user_id`

func TestCreateAndConsumeWebSocketTicket(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	mock.ExpectExec("INSERT INTO websocket_tickets").
		WithArgs("ticket-1", "user-1", "group-1", "hash", 3, now.Add(time.Minute)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateWebSocketTicket(ctx, "ticket-1", "user-1", "group-1", "hash", 3, now.Add(time.Minute)); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	// A valid ticket whose auth_version, account activity, and membership all
	// still hold is atomically consumed and returns the owning user.
	mock.ExpectQuery(consumeSQL).
		WithArgs("hash", "group-1").
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow("user-1"))
	userID, err := repo.ConsumeWebSocketTicket(ctx, "hash", "group-1")
	if err != nil || userID != "user-1" {
		t.Fatalf("consume ticket = %q, %v", userID, err)
	}

	// A consumed, missing, or expired ticket yields an empty user id instead of
	// an error so the WebSocket upgrade can reject it cleanly.
	mock.ExpectQuery(consumeSQL).
		WithArgs("hash", "group-1").
		WillReturnError(pgx.ErrNoRows)
	userID, err = repo.ConsumeWebSocketTicket(ctx, "hash", "group-1")
	if err != nil || userID != "" {
		t.Fatalf("reuse ticket = %q, %v (want empty user, no error)", userID, err)
	}
}

func TestConsumeWebSocketTicketRejectsRevokedCredentials(t *testing.T) {
	// The consume UPDATE is a single atomic statement that refuses to mark a
	// ticket used unless the user is active, the ticket's auth_version matches
	// the user's current version, and the user is still a group member. Each
	// scenario exercises the same repository contract: the database reports no
	// matching row and the repository surfaces an empty user id without an
	// error, leaving the ticket unused so it fails identically on retry.
	// The live-database semantics behind each condition are exercised by the
	// two-account integration coverage (F-03 PR B).
	repo, mock := newChatRepo(t)
	ctx := context.Background()

	for _, name := range []string{
		"auth_version bumped after issuance",
		"membership removed after issuance",
		"user deleted after issuance",
	} {
		mock.ExpectQuery(consumeSQL).WithArgs("hash", "group-1").WillReturnError(pgx.ErrNoRows)
		userID, err := repo.ConsumeWebSocketTicket(ctx, "hash", "group-1")
		if err != nil || userID != "" {
			t.Fatalf("%s: consume = %q, %v (want empty user, no error)", name, userID, err)
		}
	}
}

func TestDeleteWebSocketTickets(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 3))
	if err := repo.DeleteWebSocketTickets(ctx, "user-1"); err != nil {
		t.Fatalf("delete tickets: %v", err)
	}
}

func TestUserAuthVersion(t *testing.T) {
	repo, mock := newChatRepo(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT auth_version FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"auth_version"}).AddRow(3))
	version, err := repo.UserAuthVersion(ctx, "user-1")
	if err != nil || version != 3 {
		t.Fatalf("auth version = %d, %v", version, err)
	}

	// A missing or deleted account yields version 0 without an error so a
	// ticket bound to it can never match a live user at consumption.
	mock.ExpectQuery("SELECT auth_version FROM users").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	version, err = repo.UserAuthVersion(ctx, "missing")
	if err != nil || version != 0 {
		t.Fatalf("missing auth version = %d, %v", version, err)
	}
}
