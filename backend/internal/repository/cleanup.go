package repository

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"geoguessme/internal/database"

	"github.com/jackc/pgx/v5"
)

type RetainedMedia struct{ ID, StorageKey string }

func FindExpiredMedia(ctx context.Context, limit int) ([]RetainedMedia, error) {
	rows, err := database.DB.Query(ctx, `SELECT id, storage_key FROM photos WHERE lifecycle_status <> 'removed' AND retention_at < CURRENT_TIMESTAMP ORDER BY retention_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RetainedMedia
	for rows.Next() {
		var media RetainedMedia
		if err := rows.Scan(&media.ID, &media.StorageKey); err != nil {
			return nil, err
		}
		result = append(result, media)
	}
	return result, rows.Err()
}

func ExpireChallengeViews(ctx context.Context) error {
	_, err := database.DB.Exec(ctx, `DELETE FROM challenge_views WHERE view_expires_at < CURRENT_TIMESTAMP - interval '1 day'`)
	return err
}

// Deleter is the minimal storage capability the cleanup worker needs.
type Deleter interface {
	Delete(context.Context, string) error
}

// CleanupRunner drives token cleanup, challenge-view expiry, retention media
// deletion, and the durable object-deletion queue. It runs once immediately on
// start so a freshly booting worker clears any backlog before waiting on its
// interval.
type CleanupRunner struct {
	Store            Deleter
	Interval         time.Duration
	Logger           *slog.Logger
	Backlog          func(pending int)
	BacklogRemaining bool
}

func (r CleanupRunner) Run(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	// Clear any backlog left by a previous crash before settling into the tick.
	r.runOnce(ctx, logger)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx, logger)
		}
	}
}

func (r CleanupRunner) runOnce(ctx context.Context, logger *slog.Logger) {
	if err := CleanupAuthTokens(ctx); err != nil {
		logger.Warn("auth token cleanup failed", "error", err)
	}
	if err := ExpireChallengeViews(ctx); err != nil {
		logger.Warn("challenge view expiry failed", "error", err)
	}
	if err := r.sweepRetainedMedia(ctx, logger); err != nil {
		logger.Warn("retention media sweep failed", "error", err)
	}
	r.drainDeletionQueue(ctx, logger)
	if r.Backlog != nil {
		if count, err := CountDeletionBacklog(ctx); err != nil {
			logger.Warn("deletion backlog count failed", "error", err)
		} else {
			r.Backlog(count)
		}
	}
}

// RetireRetainedMedia atomically marks a retained media record removed and
// enqueues a durable deletion job for its object-storage key within a single
// database transaction. The target row is locked before any decision is made,
// so two concurrent sweeps can never enqueue a second job for the same object.
//
// The deletion job is created first, using the storage key captured under the
// row lock, and only then is the record marked removed with its key cleared.
// If the deletion obligation for this key already exists (a partial unique
// index conflict on an active job), the insert is a no-op and the media is still
// marked removed. If the job insert, the media update, or the commit fails the
// transaction is rolled back, leaving no committed partial state. A record
// already marked removed (or one that no longer exists) is an idempotent success
// and creates no new deletion job.
func RetireRetainedMedia(ctx context.Context, id string) error {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lifecycleStatus, storageKey string
	err = tx.QueryRow(ctx,
		`SELECT lifecycle_status, COALESCE(storage_key, '') FROM photos WHERE id = $1 FOR UPDATE`,
		id,
	).Scan(&lifecycleStatus, &storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if lifecycleStatus == "removed" {
		return nil
	}
	if storageKey != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO media_deletion_jobs(id, storage_key, source) VALUES ($1, $2, 'retention') ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING`,
			newID(), storageKey,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE photos SET lifecycle_status = 'removed', storage_key = NULL WHERE id = $1`,
		id,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// sweepRetainedMedia finds expired retained photos and retires each one through
// the atomic RetireRetainedMedia operation, which enqueues a durable deletion
// job and flips the record to removed in one transaction so a transient failure
// can never orphan the stored object. A failing record is logged and remembered
// while the remaining records are still processed; the first error propagates
// so the caller can observe partial failures.
func (r CleanupRunner) sweepRetainedMedia(ctx context.Context, logger *slog.Logger) error {
	items, err := FindExpiredMedia(ctx, 100)
	if err != nil {
		return err
	}
	var firstErr error
	for _, item := range items {
		if err := RetireRetainedMedia(ctx, item.ID); err != nil {
			logger.Error("retiring retained media failed", "photo_id", item.ID, "key", item.StorageKey, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// RunOneCycle executes a single cleanup pass (token cleanup, challenge-view
// expiry, retention media sweep, and deletion-queue drain) and is exported so
// integration tests can exercise the full one-cycle behavior without
// duplicating worker logic.
func (r CleanupRunner) RunOneCycle(ctx context.Context) {
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r.runOnce(ctx, logger)
}

// DrainDeletionQueue claims and executes pending deletion jobs in batches until
// the queue is empty or an error stops progress. Exported so focused
// deletion-retry tests can exercise it with a controlled Deleter without
// duplicating worker logic.
func (r CleanupRunner) DrainDeletionQueue(ctx context.Context) {
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r.drainDeletionQueue(ctx, logger)
}

func (r CleanupRunner) drainDeletionQueue(ctx context.Context, logger *slog.Logger) {
	for {
		jobs, err := ClaimDeletionJobs(ctx, 25, 15*time.Minute)
		if err != nil {
			logger.Warn("claiming deletion jobs failed", "error", err)
			return
		}
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			if err := r.Store.Delete(ctx, job.StorageKey); err != nil {
				logger.Warn("object deletion failed", "job_id", job.ID, "key", job.StorageKey, "attempt", job.Attempts, "error", err)
				_ = FailDeletionJob(ctx, job.ID, err.Error())
				continue
			}
			if err := CompleteDeletionJob(ctx, job.ID); err != nil {
				logger.Warn("marking deletion job complete failed", "job_id", job.ID, "error", err)
			}
		}
	}
}
