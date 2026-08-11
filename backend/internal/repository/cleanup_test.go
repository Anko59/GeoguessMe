package repository

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func newCleanupMock(t *testing.T) pgxmock.PgxPoolIface {
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

func TestExpirePushSubscriptionsDeletesIdleRows(t *testing.T) {
	mock := newCleanupMock(t)
	mock.ExpectExec("DELETE FROM push_subscriptions WHERE COALESCE\\(last_used_at, created_at\\) < \\$1").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 3))
	if err := (&Repository{pool: mock}).ExpirePushSubscriptions(context.Background(), 90*24*time.Hour); err != nil {
		t.Fatalf("ExpirePushSubscriptions error = %v", err)
	}
}
