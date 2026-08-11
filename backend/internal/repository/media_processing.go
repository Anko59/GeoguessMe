package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// execer is satisfied by both the connection pool and a transaction. It lets
// the job-completion UPDATE run against either so the media-processing worker
// can complete a job inside the same transaction that creates its result
// record.
type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// processingJobColumns lists every scanned column in storage order. Keep in
// sync with the SELECTs below and with scanProcessingJob.
const processingJobColumns = `id, user_id, kind, status, quarantine_key, canonical_key, result_kind, result_id, mime_type, byte_size, error_code, group_id, metadata, worker_id, queued_at, started_at, completed_at, expires_at`

// CreateProcessingJob records a new queued job for a quarantined upload. The
// caller owns the opaque metadata JSON; the worker needs it to reconstruct the
// challenge or chat record once the canonical object exists.
func (r *Repository) CreateProcessingJob(ctx context.Context, job *models.MediaProcessingJob) error {
	query := `INSERT INTO media_processing_jobs (id, user_id, kind, status, quarantine_key, group_id, metadata, worker_id, queued_at, expires_at)
		VALUES ($1, $2, $3, 'queued', $4, $5, $6, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + interval '24 hours')`
	// The metadata JSON must be sent as jsonb, not bytea: pgx encodes a plain
	// []byte parameter as bytea, which Postgres rejects for a jsonb column.
	// json.RawMessage is encoded as its raw JSON text and type-infers to jsonb.
	_, err := r.pool.Exec(ctx, query, job.ID, job.UserID, job.Kind, job.QuarantineKey, job.GroupID, json.RawMessage(job.Metadata))
	return err
}

// ClaimProcessingJob atomically claims one queued job for a worker. The
// UPDATE ... FOR UPDATE SKIP LOCKED subquery makes concurrent claims
// non-colliding: each worker sees a distinct job or none. It returns (nil, nil)
// when no queued job is available.
func (r *Repository) ClaimProcessingJob(ctx context.Context, workerID string) (*models.MediaProcessingJob, error) {
	query := `UPDATE media_processing_jobs
		SET status = 'processing', started_at = CURRENT_TIMESTAMP, worker_id = $1
		WHERE id = (SELECT id FROM media_processing_jobs WHERE status = 'queued' ORDER BY queued_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING ` + processingJobColumns
	job, err := scanProcessingJob(r.pool.QueryRow(ctx, query, workerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

// CompleteProcessingJob marks a claimed job ready with its canonical result.
func (r *Repository) CompleteProcessingJob(ctx context.Context, jobID, leaseToken, canonicalKey, resultKind, resultID, mimeType string, byteSize int64) error {
	return r.completeProcessingJob(ctx, r.pool, jobID, leaseToken, canonicalKey, resultKind, resultID, mimeType, byteSize)
}

// CompleteProcessingJobTx marks a job ready using the supplied transaction. It
// is the tx-scoped variant used by the media-processing worker so the result
// record and the job completion commit atomically.
func (r *Repository) CompleteProcessingJobTx(ctx context.Context, tx pgx.Tx, jobID, leaseToken, canonicalKey, resultKind, resultID, mimeType string, byteSize int64) error {
	return r.completeProcessingJob(ctx, tx, jobID, leaseToken, canonicalKey, resultKind, resultID, mimeType, byteSize)
}

func (r *Repository) completeProcessingJob(ctx context.Context, q execer, jobID, leaseToken, canonicalKey, resultKind, resultID, mimeType string, byteSize int64) error {
	tag, err := q.Exec(ctx, `UPDATE media_processing_jobs
		SET status = 'ready', canonical_key = $2, result_kind = $3, result_id = $4, mime_type = $5, byte_size = $6, completed_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'processing' AND worker_id = $7`, jobID, canonicalKey, resultKind, resultID, mimeType, byteSize, leaseToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return models.ErrMediaProcessingLeaseLost
	}
	return nil
}

// CompleteChallengeProcessing marks a job ready and inserts its per-group
// challenge photos in a single transaction. The worker writes the canonical
// objects first; this transaction records the photo rows and the job result
// atomically, so a failure can never leave a ready job without its records or
// records without a ready job. The job's canonical key and result id point at
// the first photo.
func (r *Repository) CompleteChallengeProcessing(ctx context.Context, jobID, leaseToken, quarantineKey string, photos []*models.Photo, mimeType string, byteSize int64) error {
	if len(photos) == 0 {
		return errors.New("challenge processing requires at least one photo")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	groupIDs := make([]string, 0, len(photos))
	for _, photo := range photos {
		groupIDs = append(groupIDs, photo.GroupID)
	}
	var authorized bool
	if err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM unnest($2::text[]) AS target(group_id)
		WHERE NOT EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = target.group_id AND gm.user_id = $1)
	)`, photos[0].UserID, groupIDs).Scan(&authorized); err != nil {
		return err
	}
	if !authorized {
		return models.ErrMediaProcessingAuthorizationRevoked
	}
	if err := r.Groups.CreatePhotosTx(ctx, tx, photos); err != nil {
		return err
	}
	if err := r.completeProcessingJob(ctx, tx, jobID, leaseToken, photos[0].StorageKey, "photo", photos[0].ID, mimeType, byteSize); err != nil {
		return err
	}
	if err := enqueueMediaDeletionTx(ctx, tx, "media-processing", quarantineKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CompleteChatProcessing marks a job ready and inserts its chat message and
// attachment in a single transaction. On success msg carries the media
// reference the caller needs for the realtime broadcast.
func (r *Repository) CompleteChatProcessing(ctx context.Context, jobID, leaseToken, quarantineKey string, msg *models.Message, asset *models.ChatMedia) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var authorized bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, msg.GroupID, msg.UserID).Scan(&authorized); err != nil {
		return err
	}
	if !authorized {
		return models.ErrMediaProcessingAuthorizationRevoked
	}
	if err := r.Chat.CreateChatMediaMessageTx(ctx, tx, msg, asset); err != nil {
		return err
	}
	if err := r.completeProcessingJob(ctx, tx, jobID, leaseToken, asset.StorageKey, "chat", msg.ID, asset.MIMEType, asset.ByteSize); err != nil {
		return err
	}
	if err := enqueueMediaDeletionTx(ctx, tx, "media-processing", quarantineKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FailClaimedProcessingJob fails only the caller's live claim. A stale worker
// cannot fail or delete the source of a job that has already been reclaimed.
func (r *Repository) FailClaimedProcessingJob(ctx context.Context, jobID, leaseToken, quarantineKey, errorCode string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE media_processing_jobs
		SET status = 'failed', error_code = $2, completed_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'processing' AND worker_id = $3`, jobID, errorCode, leaseToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return models.ErrMediaProcessingLeaseLost
	}
	if err := enqueueMediaDeletionTx(ctx, tx, "media-processing", quarantineKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetProcessingJob returns a job by id for its owner. A non-owner or missing
// job yields (nil, nil) so the endpoint can answer a uniform not-found.
func (r *Repository) GetProcessingJob(ctx context.Context, jobID, userID string) (*models.MediaProcessingJob, error) {
	query := `SELECT ` + processingJobColumns + ` FROM media_processing_jobs WHERE id = $1 AND user_id = $2`
	job, err := scanProcessingJob(r.pool.QueryRow(ctx, query, jobID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

// RequeueStaleProcessingJobs returns processing jobs whose worker died
// (started_at older than staleAfter) to the queued state so a fresh worker can
// pick them up. It returns the number of jobs requeued.
func (r *Repository) RequeueStaleProcessingJobs(ctx context.Context, staleAfter time.Duration) (int, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE media_processing_jobs
		SET status = 'queued', started_at = NULL, worker_id = NULL
		WHERE status = 'processing' AND started_at < CURRENT_TIMESTAMP - $1::interval`, staleAfter)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// PurgeExpiredProcessingJobs deletes finished (ready/failed) jobs whose
// 24-hour retention window has passed. Raw quarantine objects are cleaned up
// separately through AbandonedQuarantine. It returns the number purged.
func (r *Repository) PurgeExpiredProcessingJobs(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM media_processing_jobs
		WHERE status IN ('ready', 'failed') AND expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// AbandonedQuarantine is a quarantined upload whose job never reached ready,
// eligible for durable deletion by the cleanup runner.
type AbandonedQuarantine struct {
	JobID         string
	QuarantineKey string
}

// AbandonQueuedProcessingJobs atomically transitions old queued jobs to failed
// and returns their quarantine keys. Claiming and abandonment can therefore
// never select the same job, and active workers are never cleaned underneath.
func (r *Repository) AbandonQueuedProcessingJobs(ctx context.Context, abandonAfter time.Duration, errorCode string) ([]AbandonedQuarantine, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `UPDATE media_processing_jobs
		SET status = 'failed', error_code = $2, completed_at = CURRENT_TIMESTAMP
		WHERE status = 'queued' AND queued_at < CURRENT_TIMESTAMP - $1::interval
		RETURNING id, quarantine_key`, abandonAfter, errorCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AbandonedQuarantine, 0)
	for rows.Next() {
		var item AbandonedQuarantine
		if err := rows.Scan(&item.JobID, &item.QuarantineKey); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, item := range result {
		if err := enqueueMediaDeletionTx(ctx, tx, "media-processing-abandoned", item.QuarantineKey); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func enqueueMediaDeletionTx(ctx context.Context, tx execer, source, key string) error {
	_, err := tx.Exec(ctx, `INSERT INTO media_deletion_jobs(id, storage_key, source) VALUES ($1, $2, $3) ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING`, newID(), key, source)
	return err
}

func scanProcessingJob(row rowScanner) (*models.MediaProcessingJob, error) {
	var job models.MediaProcessingJob
	// Queued jobs leave the canonical-result and worker columns NULL until the
	// worker claims or completes them; challenge jobs have no single group id.
	// Scan the nullable columns into database/sql nullable types (the same
	// convention chat/scan.go uses) so a NULL scan never turns into an error.
	var canonicalKey, resultKind, resultID, mimeType, errorCode, groupID, workerID sql.NullString
	var byteSize sql.NullInt64
	err := row.Scan(
		&job.ID, &job.UserID, &job.Kind, &job.Status, &job.QuarantineKey,
		&canonicalKey, &resultKind, &resultID, &mimeType,
		&byteSize, &errorCode, &groupID, &job.Metadata,
		&workerID, &job.QueuedAt, &job.StartedAt, &job.CompletedAt, &job.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	job.CanonicalKey = canonicalKey.String
	job.ResultKind = resultKind.String
	job.ResultID = resultID.String
	job.MIMEType = mimeType.String
	job.ByteSize = byteSize.Int64
	job.ErrorCode = errorCode.String
	job.GroupID = groupID.String
	job.WorkerID = workerID.String
	return &job, nil
}
