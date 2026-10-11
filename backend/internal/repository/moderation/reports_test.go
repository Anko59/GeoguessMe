package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

func TestContentReportPersistenceAndAuthorization(t *testing.T) {
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
	repo := NewRepository(mock)
	ctx := context.Background()
	if _, err := repo.CreateContentReport(ctx, "reporter", "photo", "target-id", "other", "context"); !errors.Is(err, ErrReportTargetUnavailable) {
		t.Fatalf("unsupported target kind: %v", err)
	}
	for _, tc := range []struct {
		name    string
		kind    string
		id      any
		created bool
		wantErr error
	}{
		{"new message", "message", "report-1", true, nil},
		{"duplicate user", "user", "report-2", false, nil},
		{"missing or inaccessible", "message", nil, false, ErrReportTargetUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := pgxmock.NewRows([]string{"id", "reported_user_id", "created"})
			if tc.wantErr == nil {
				rows.AddRow(tc.id, "target-user", tc.created)
			}
			mock.ExpectQuery("WITH eligible AS \\((?s:.*JOIN group_members gm.*INSERT INTO content_reports.*)ON CONFLICT \\(reporter_id, target_kind, target_id\\) DO NOTHING").
				WithArgs("reporter", tc.kind, "target-id", "other", "context", pgxmock.AnyArg()).WillReturnRows(rows)
			got, err := repo.CreateContentReport(ctx, "reporter", tc.kind, "target-id", "other", "context")
			if !errors.Is(err, tc.wantErr) || (err == nil && (got.ID != tc.id || got.Created != tc.created)) {
				t.Fatalf("CreateContentReport = %+v, %v", got, err)
			}
		})
	}
	// A conflicting insert may commit after the statement snapshot: the next
	// statement sees its ID without sending a duplicate notification.
	mock.ExpectQuery("WITH eligible AS").WithArgs("reporter", "message", "target-id", "other", "context", pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows([]string{"id", "reported_user_id", "created"}).AddRow(nil, "target-user", false))
	mock.ExpectQuery("SELECT id FROM content_reports").WithArgs("reporter", "message", "target-id").WillReturnRows(
		pgxmock.NewRows([]string{"id"}).AddRow("winning-report"))
	got, err := repo.CreateContentReport(ctx, "reporter", "message", "target-id", "other", "context")
	if err != nil || got.ID != "winning-report" || got.Created {
		t.Fatalf("concurrent duplicate = %+v, %v", got, err)
	}
	mock.ExpectQuery("WITH eligible AS").WithArgs("reporter", "message", "target-id", "other", "context", pgxmock.AnyArg()).WillReturnError(pgx.ErrTxClosed)
	if _, err := repo.CreateContentReport(ctx, "reporter", "message", "target-id", "other", "context"); !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("database error = %v", err)
	}
}
