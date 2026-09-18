package groups

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

// TestRequireMemberCentralizesMembership pins the canonical membership gate
// that every gameplay handler delegates to (roadmap PR 6 item E): a member
// passes, a non-member gets the shared ErrNotMember sentinel, and a
// persistence failure propagates untouched.
func TestRequireMemberCentralizesMembership(t *testing.T) {
	ctx := context.Background()

	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if err := repo.RequireMember(ctx, "g1", "u1"); err != nil {
		t.Fatalf("member gate = %v, want nil", err)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if err := repo.RequireMember(ctx, "g1", "u2"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("non-member gate = %v, want ErrNotMember", err)
	}

	dbErr := errors.New("connection lost")
	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u3").WillReturnError(dbErr)
	if err := repo.RequireMember(ctx, "g1", "u3"); !errors.Is(err, dbErr) {
		t.Fatalf("persistence failure gate = %v, want %v", err, dbErr)
	}
}

func TestUserInboxReturnsUnreadCountAndLatestMetadata(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT g.id, g.name").WithArgs("viewer").WillReturnRows(
		pgxmock.NewRows([]string{"id", "name", "unread", "message_id", "kind", "username", "message_at"}).
			AddRow("group-1", "Paris", int64(3), "message-1", "text", "Alice", now).
			AddRow("group-2", "No chat", int64(0), nil, nil, nil, nil),
	)

	inbox, err := repo.UserInbox(context.Background(), "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 || inbox[0].UnreadCount != 3 || inbox[0].LatestMessage == nil || inbox[0].LatestMessage.Username != "Alice" {
		t.Fatalf("inbox = %+v", inbox)
	}
	if inbox[1].LatestMessage != nil {
		t.Fatalf("empty group has latest message: %+v", inbox[1].LatestMessage)
	}
}

func TestMarkInboxReadRequiresMembershipAndIsMonotonic(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec("INSERT INTO group_message_reads").WithArgs("group-1", "viewer", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.MarkInboxRead(context.Background(), "group-1", "viewer", now); err != nil {
		t.Fatalf("mark read = %v", err)
	}
	mock.ExpectExec("INSERT INTO group_message_reads").WithArgs("group-2", "outsider", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	if err := repo.MarkInboxRead(context.Background(), "group-2", "outsider", now); !errors.Is(err, ErrNotMember) {
		t.Fatalf("outsider mark read = %v, want ErrNotMember", err)
	}
}
