package repository

import (
	"context"
	"geoguessme/internal/database"
	"github.com/pashagolub/pgxmock/v4"
	"log/slog"
	"testing"
	"time"
)

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

type recordingDeleter struct {
	keys []string
}

func (d *recordingDeleter) Delete(_ context.Context, key string) error {
	d.keys = append(d.keys, key)
	return nil
}

func TestDeletionQueueAndCleanupQueries(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	if err := EnqueueMediaDeletion(ctx, "test", nil); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/a", "test").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "photos/b", "test").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := EnqueueMediaDeletion(ctx, "test", []string{"photos/a", "photos/b"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE media_deletion_jobs").WithArgs(15*time.Minute, 25).WillReturnRows(pgxmock.NewRows([]string{"id", "storage_key", "attempts"}).AddRow("job-1", "photos/a", 1))
	mock.ExpectCommit()
	jobs, err := ClaimDeletionJobs(ctx, 25, 15*time.Minute)
	if err != nil || len(jobs) != 1 || jobs[0].StorageKey != "photos/a" {
		t.Fatalf("claimed jobs = %+v, %v", jobs, err)
	}
	mock.ExpectExec("UPDATE media_deletion_jobs SET completed_at").WithArgs("job-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE media_deletion_jobs SET last_error").WithArgs("temporary", "job-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := CompleteDeletionJob(ctx, "job-1"); err != nil {
		t.Fatal(err)
	}
	if err := FailDeletionJob(ctx, "job-1", "temporary"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM media_deletion_jobs").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	count, err := CountDeletionBacklog(ctx)
	if err != nil || count != 2 {
		t.Fatalf("backlog = %d, %v", count, err)
	}

	mock.ExpectQuery("SELECT id, storage_key FROM photos").WithArgs(10).WillReturnRows(pgxmock.NewRows([]string{"id", "storage_key"}).AddRow("photo-1", "photos/one"))
	items, err := FindExpiredMedia(ctx, 10)
	if err != nil || len(items) != 1 || items[0].ID != "photo-1" {
		t.Fatalf("expired media = %+v, %v", items, err)
	}
	mock.ExpectExec("UPDATE photos SET lifecycle_status").WithArgs("photo-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM challenge_views").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := MarkMediaRemoved(ctx, "photo-1"); err != nil {
		t.Fatal(err)
	}
	if err := ExpireChallengeViews(ctx); err != nil {
		t.Fatal(err)
	}

	deleter := &recordingDeleter{}
	mock.ExpectExec("DELETE FROM refresh_sessions").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM challenge_views").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectQuery("SELECT id, storage_key FROM photos").WithArgs(100).WillReturnRows(pgxmock.NewRows([]string{"id", "storage_key"}))
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE media_deletion_jobs").WithArgs(15*time.Minute, 25).WillReturnRows(pgxmock.NewRows([]string{"id", "storage_key", "attempts"}))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM media_deletion_jobs").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	CleanupRunner{Store: deleter, Interval: time.Hour, Backlog: func(int) {}}.runOnce(ctx, slog.Default())
	if len(deleter.keys) != 0 {
		t.Fatalf("unexpected cleanup deletions: %v", deleter.keys)
	}
}

func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = mock
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		database.DB = previous
	})
	return mock
}

func TestMessageCursorRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!", "++++", "onlyonepart", "abc|"} {
		if _, _, err := decodeMessageCursor(bad); err == nil {
			t.Errorf("expected error for cursor %q", bad)
		}
	}
}
