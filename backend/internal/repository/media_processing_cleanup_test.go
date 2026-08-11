package repository

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

// TestSweepAbandonedProcessing proves the abandoned-quarantine sweep deletes
// each raw object older than one hour and fails the owning job with the stable
// timeout code (matching mediaprocessing.ErrorTimeout).
func TestSweepAbandonedProcessing(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	deleter := &recordingDeleter{}

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE media_processing_jobs.*status = 'failed'").
		WithArgs(time.Hour, "timeout").
		WillReturnRows(pgxmock.NewRows([]string{"id", "quarantine_key"}).
			AddRow("job-1", "quarantine/raw-1").
			AddRow("job-2", "quarantine/raw-2"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "quarantine/raw-1", "media-processing-abandoned").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "quarantine/raw-2", "media-processing-abandoned").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	if err := (CleanupRunner{Store: deleter, Repos: repo}).sweepAbandonedProcessing(ctx, slog.Default()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(deleter.keys) != 2 || deleter.keys[0] != "quarantine/raw-1" || deleter.keys[1] != "quarantine/raw-2" {
		t.Fatalf("deleted keys = %v", deleter.keys)
	}
}

// TestSweepAbandonedProcessingEnqueuesOnDeleteFailure proves an object that
// refuses immediate deletion becomes a durable deletion job instead of being
// dropped, while the job is still failed.
func TestSweepAbandonedProcessingEnqueuesOnDeleteFailure(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	deleter := &failingDeleter{}

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE media_processing_jobs.*status = 'failed'").
		WithArgs(time.Hour, "timeout").
		WillReturnRows(pgxmock.NewRows([]string{"id", "quarantine_key"}).
			AddRow("job-1", "quarantine/raw-1"))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "quarantine/raw-1", "media-processing-abandoned").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO media_deletion_jobs").
		WithArgs(pgxmock.AnyArg(), "quarantine/raw-1", "media-processing-abandoned").
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	mock.ExpectCommit()

	if err := (CleanupRunner{Store: deleter, Repos: repo}).sweepAbandonedProcessing(ctx, slog.Default()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
}

// failingDeleter always fails deletes so the sweep must fall back to the
// durable deletion queue.
type failingDeleter struct{}

func (failingDeleter) Delete(context.Context, string) error {
	return errors.New("storage unavailable")
}
