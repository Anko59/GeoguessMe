package repository

import (
	"context"
	"log/slog"
	"time"

	"geoguessme/internal/database"
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

// MarkMediaRemoved nulls the storage key once retention cleanup has enqueued a
// deletion job, preventing the same object being queued twice.
func MarkMediaRemoved(ctx context.Context, id string) error {
	_, err := database.DB.Exec(ctx, `UPDATE photos SET lifecycle_status = 'removed', storage_key = NULL WHERE id = $1`, id)
	return err
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

// sweepRetainedMedia finds expired retained photos and enqueues durable
// deletion jobs rather than deleting objects inline, so a transient storage
// outage cannot lose the cleanup obligation.
func (r CleanupRunner) sweepRetainedMedia(ctx context.Context, logger *slog.Logger) error {
	items, err := FindExpiredMedia(ctx, 100)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := MarkMediaRemoved(ctx, item.ID); err != nil {
			logger.Warn("marking retained media removed failed", "photo_id", item.ID, "error", err)
			continue
		}
		if err := EnqueueMediaDeletion(ctx, "retention", []string{item.StorageKey}); err != nil {
			logger.Error("enqueuing retained media deletion failed", "photo_id", item.ID, "key", item.StorageKey, "error", err)
		}
	}
	return nil
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
