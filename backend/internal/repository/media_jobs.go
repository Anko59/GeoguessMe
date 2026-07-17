package repository

import (
	"context"
	"time"

	"geoguessme/internal/database"

	"github.com/jackc/pgx/v5"
)

// EnqueueMediaDeletion records durable deletion jobs for object-storage keys.
// Used by account/group deletion so media can never be orphaned.
func EnqueueMediaDeletion(ctx context.Context, source string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, key := range keys {
		if _, err := tx.Exec(ctx, `INSERT INTO media_deletion_jobs(id, storage_key, source) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, newID(), key, source); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ClaimedMediaJob is a deletion job handed to the cleanup worker for one
// attempt. A job is claimed atomically so concurrent workers never collide.
type ClaimedMediaJob struct {
	ID         string
	StorageKey string
	Attempts   int
}

// ClaimDeletionJobs reserves up to limit pending jobs for the caller by pushing
// their next_attempt_at into the future, preventing immediate re-claim.
func ClaimDeletionJobs(ctx context.Context, limit int, backoff time.Duration) ([]ClaimedMediaJob, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE media_deletion_jobs
		SET attempts = attempts + 1, next_attempt_at = CURRENT_TIMESTAMP + $1
		WHERE id IN (SELECT id FROM media_deletion_jobs WHERE completed_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP ORDER BY next_attempt_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		RETURNING id, storage_key, attempts`, backoff, limit)
	if err != nil {
		return nil, err
	}
	jobs := make([]ClaimedMediaJob, 0)
	for rows.Next() {
		var job ClaimedMediaJob
		if err := rows.Scan(&job.ID, &job.StorageKey, &job.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return jobs, nil
}

func CompleteDeletionJob(ctx context.Context, id string) error {
	_, err := database.DB.Exec(ctx, `UPDATE media_deletion_jobs SET completed_at = CURRENT_TIMESTAMP, last_error = NULL WHERE id = $1`, id)
	return err
}

func FailDeletionJob(ctx context.Context, id, message string) error {
	_, err := database.DB.Exec(ctx, `UPDATE media_deletion_jobs SET last_error = $1 WHERE id = $2`, message, id)
	return err
}

// CountDeletionBacklog reports jobs awaiting completion (for health/metrics).
func CountDeletionBacklog(ctx context.Context) (int, error) {
	var count int
	err := database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM media_deletion_jobs WHERE completed_at IS NULL`).Scan(&count)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	return count, nil
}
