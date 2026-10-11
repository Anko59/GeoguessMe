package blocking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

func TestDirectedIdempotentPreferences(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	repo := NewRepository(mock)
	ctx := context.Background()
	if !errors.Is(repo.Block(ctx, "owner", "owner"), ErrUnavailable) {
		t.Fatal("self block")
	}
	for _, affected := range []int64{1, 1, 0} {
		mock.ExpectExec(`INSERT INTO user_blocks(?s:.*)ON CONFLICT \(blocker_id, blocked_id\) DO UPDATE SET created_at=user_blocks.created_at`).WithArgs("owner", "target").WillReturnResult(pgxmock.NewResult("INSERT", affected))
		err := repo.Block(ctx, "owner", "target")
		if affected == 0 && !errors.Is(err, ErrUnavailable) || affected == 1 && err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectExec(`DELETE FROM user_blocks WHERE blocker_id=\$1 AND blocked_id=\$2`).WithArgs("owner", "target").WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := repo.Unblock(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSymmetricVisibilityFailsClosed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	repo := NewRepository(mock)
	ctx := context.Background()
	for _, blocked := range []bool{false, true} {
		mock.ExpectQuery(`SELECT EXISTS(?s:.*)blocker_id=\$2 AND blocked_id=\$1`).WithArgs("viewer", "target").WillReturnRows(pgxmock.NewRows([]string{"blocked"}).AddRow(blocked))
		got, err := repo.Blocked(ctx, "viewer", "target")
		if err != nil || got != blocked {
			t.Fatalf("%v %v", got, err)
		}
	}
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs("viewer", "target").WillReturnError(pgx.ErrTxClosed)
	if _, err := repo.Blocked(ctx, "viewer", "target"); !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	if got, err := repo.Blocked(ctx, "viewer", "viewer"); err != nil || got {
		t.Fatal("self hidden")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateIdentityOnlyList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT b.blocked_id,u.username,u.avatar,b.created_at(?s:.*)WHERE b.blocker_id=\$1 ORDER BY`).WithArgs("owner").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "created_at"}).AddRow("target", "name", "custom", now))
	items, err := NewRepository(mock).List(context.Background(), "owner")
	if err != nil || len(items) != 1 || items[0].UserID != "target" || items[0].CreatedAt != now {
		t.Fatalf("%+v %v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
