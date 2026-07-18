package repository

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"geoguessme/internal/database"

	"github.com/pashagolub/pgxmock/v4"
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
	// RetireRetainedMedia replaces the old MarkMediaRemoved API with the
	// atomic lock→job→update flow.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs("photo-1").
		WillReturnRows(retainedMediaRow("ready", "photos/one"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/one").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE photos SET lifecycle_status").
		WithArgs("photo-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	if err := RetireRetainedMedia(ctx, "photo-1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM challenge_views").WillReturnResult(pgxmock.NewResult("DELETE", 1))
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

// retainedMediaRow builds the row produced by the locked read inside
// RetireRetainedMedia. pgxmock scans positionally, so the column names are only
// for readability.
func retainedMediaRow(lifecycleStatus, storageKey string) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"lifecycle_status", "storage_key"}).AddRow(lifecycleStatus, storageKey)
}

// TestRetireRetainedMediaSuccess verifies the ordered transaction: lock and
// read the row, insert the deletion job with the original storage key, mark the
// media removed and clear its key, then commit.
func TestRetireRetainedMediaSuccess(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	const photoID = "photo-1"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs(photoID).
		WillReturnRows(retainedMediaRow("ready", "photos/original"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/original").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE photos SET lifecycle_status").
		WithArgs(photoID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	if err := RetireRetainedMedia(ctx, photoID); err != nil {
		t.Fatalf("retire retained media: %v", err)
	}
}

// TestRetireRetainedMediaRollsBackOnJobInsertFailure ensures a deletion-job
// insert failure aborts the transaction: the rollback is explicit, no media
// update and no commit occur.
func TestRetireRetainedMediaRollsBackOnJobInsertFailure(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	const photoID = "photo-1"
	jobErr := errors.New("job insert failed")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs(photoID).
		WillReturnRows(retainedMediaRow("ready", "photos/original"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/original").
		WillReturnError(jobErr)
	mock.ExpectRollback()
	if err := RetireRetainedMedia(ctx, photoID); !errors.Is(err, jobErr) {
		t.Fatalf("expected job insert error to propagate, got %v", err)
	}
}

// TestRetireRetainedMediaRollsBackOnMediaUpdateFailure ensures a media-update
// failure aborts the transaction after the job was inserted: the rollback is
// explicit and nothing commits.
func TestRetireRetainedMediaRollsBackOnMediaUpdateFailure(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	const photoID = "photo-1"
	updateErr := errors.New("media update failed")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs(photoID).
		WillReturnRows(retainedMediaRow("ready", "photos/original"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/original").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE photos SET lifecycle_status").
		WithArgs(photoID).
		WillReturnError(updateErr)
	mock.ExpectRollback()
	if err := RetireRetainedMedia(ctx, photoID); !errors.Is(err, updateErr) {
		t.Fatalf("expected media update error to propagate, got %v", err)
	}
}

// TestRetireRetainedMediaCommitFailure ensures a commit failure surfaces an
// error after both writes succeeded within the transaction.
func TestRetireRetainedMediaCommitFailure(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	const photoID = "photo-1"
	commitErr := errors.New("commit failed")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs(photoID).
		WillReturnRows(retainedMediaRow("ready", "photos/original"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/original").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE photos SET lifecycle_status").
		WithArgs(photoID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit().WillReturnError(commitErr)
	if err := RetireRetainedMedia(ctx, photoID); !errors.Is(err, commitErr) {
		t.Fatalf("expected commit error to propagate, got %v", err)
	}
}

// TestRetireRetainedMediaAlreadyRemovedIsIdempotent ensures an already-removed
// record is a no-op success: no deletion job is enqueued and nothing is updated.
func TestRetireRetainedMediaAlreadyRemovedIsIdempotent(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	const photoID = "photo-1"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs(photoID).
		WillReturnRows(retainedMediaRow("removed", ""))
	if err := RetireRetainedMedia(ctx, photoID); err != nil {
		t.Fatalf("already-removed media should retire idempotently, got %v", err)
	}
}

// TestSweepRetainedMediaUsesAtomicOperationAndPropagatesError ensures the
// cleanup service drives retained media through RetireRetainedMedia only and
// surfaces its error instead of swallowing it.
func TestSweepRetainedMediaUsesAtomicOperationAndPropagatesError(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	jobErr := errors.New("job insert failed")
	mock.ExpectQuery("SELECT id, storage_key FROM photos").
		WithArgs(100).
		WillReturnRows(pgxmock.NewRows([]string{"id", "storage_key"}).AddRow("photo-1", "photos/original"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT lifecycle_status, COALESCE.*FOR UPDATE").
		WithArgs("photo-1").
		WillReturnRows(retainedMediaRow("ready", "photos/original"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "photos/original").
		WillReturnError(jobErr)
	runner := CleanupRunner{Store: &recordingDeleter{}, Interval: time.Hour}
	if err := runner.sweepRetainedMedia(ctx, slog.Default()); !errors.Is(err, jobErr) {
		t.Fatalf("expected sweep to propagate retire error, got %v", err)
	}
}

func TestMessageCursorRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!", "++++", "onlyonepart", "abc|"} {
		if _, _, err := decodeMessageCursor(bad); err == nil {
			t.Errorf("expected error for cursor %q", bad)
		}
	}
}
