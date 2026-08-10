package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

// processingJobRow builds the row produced by the job SELECTs/RETURNING.
// pgxmock scans positionally, so the column names are only for readability.
func processingJobRow(job *models.MediaProcessingJob) *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "user_id", "kind", "status", "quarantine_key", "canonical_key",
		"result_kind", "result_id", "mime_type", "byte_size", "error_code",
		"group_id", "metadata", "worker_id", "queued_at", "started_at",
		"completed_at", "expires_at",
	}).AddRow(job.ID, job.UserID, job.Kind, job.Status, job.QuarantineKey,
		job.CanonicalKey, job.ResultKind, job.ResultID, job.MIMEType, job.ByteSize,
		job.ErrorCode, job.GroupID, job.Metadata, job.WorkerID, job.QueuedAt,
		job.StartedAt, job.CompletedAt, job.ExpiresAt)
}

func sampleProcessingJob() *models.MediaProcessingJob {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	return &models.MediaProcessingJob{
		ID:            "job-1",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChallenge,
		Status:        models.MediaProcessingStatusProcessing,
		QuarantineKey: "quarantine/raw-uuid",
		CanonicalKey:  "photos/canonical-uuid",
		ResultKind:    "challenge",
		ResultID:      "photo-1",
		MIMEType:      "video/mp4",
		ByteSize:      1234,
		GroupID:       "group-1",
		Metadata:      []byte(`{"lat":1.5,"long":2.5}`),
		WorkerID:      "worker-1",
		QueuedAt:      now.Add(-time.Minute),
		StartedAt:     ptrTime(now),
		CompletedAt:   nil,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestCreateProcessingJob(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	job := &models.MediaProcessingJob{
		ID:            "job-1",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChat,
		QuarantineKey: "quarantine/raw-uuid",
		GroupID:       "group-1",
		Metadata:      []byte(`{"reply_to":"m-1"}`),
	}
	mock.ExpectExec("INSERT INTO media_processing_jobs").
		WithArgs(job.ID, job.UserID, job.Kind, job.QuarantineKey, job.GroupID, json.RawMessage(job.Metadata)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateProcessingJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func TestClaimProcessingJobReturnsJobThenNoRows(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	job := sampleProcessingJob()

	// First claim: one queued job is atomically reserved and returned.
	mock.ExpectQuery("UPDATE media_processing_jobs.*SKIP LOCKED").
		WithArgs("worker-1").
		WillReturnRows(processingJobRow(job))
	got, err := repo.ClaimProcessingJob(ctx, "worker-1")
	if err != nil || got == nil || got.ID != job.ID || got.Status != models.MediaProcessingStatusProcessing {
		t.Fatalf("first claim = %+v, %v", got, err)
	}
	if got.QuarantineKey != "quarantine/raw-uuid" || got.Metadata == nil {
		t.Fatalf("claimed job lost quarantine/metadata: %+v", got)
	}

	// Second concurrent claim: the only job was already reserved, so no row is
	// returned and the claim is a nil, nil no-op rather than an error.
	mock.ExpectQuery("UPDATE media_processing_jobs.*SKIP LOCKED").
		WithArgs("worker-2").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "kind", "status", "quarantine_key", "canonical_key",
			"result_kind", "result_id", "mime_type", "byte_size", "error_code",
			"group_id", "metadata", "worker_id", "queued_at", "started_at",
			"completed_at", "expires_at",
		}))
	got, err = repo.ClaimProcessingJob(ctx, "worker-2")
	if err != nil || got != nil {
		t.Fatalf("second claim = %+v, %v (want nil, nil)", got, err)
	}
}

func TestCompleteAndFailProcessingJob(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()

	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'ready'").
		WithArgs("job-1", "photos/canonical-uuid", "challenge", "photo-1", "video/mp4", int64(1234)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.CompleteProcessingJob(ctx, "job-1", "photos/canonical-uuid", "challenge", "photo-1", "video/mp4", 1234); err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'failed'").
		WithArgs("job-2", "transcode_failed").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.FailProcessingJob(ctx, "job-2", "transcode_failed"); err != nil {
		t.Fatal(err)
	}
}

func TestGetProcessingJobIsOwnerScoped(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	job := sampleProcessingJob()

	mock.ExpectQuery("SELECT .* FROM media_processing_jobs WHERE id = \\$1 AND user_id = \\$2").
		WithArgs("job-1", "user-1").
		WillReturnRows(processingJobRow(job))
	got, err := repo.GetProcessingJob(ctx, "job-1", "user-1")
	if err != nil || got == nil || got.ID != job.ID {
		t.Fatalf("owner fetch = %+v, %v", got, err)
	}

	// A non-owner (or missing job) yields nil, nil: the endpoint answers a
	// uniform not-found without confirming the job exists.
	mock.ExpectQuery("SELECT .* FROM media_processing_jobs WHERE id = \\$1 AND user_id = \\$2").
		WithArgs("job-1", "user-2").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "kind", "status", "quarantine_key", "canonical_key",
			"result_kind", "result_id", "mime_type", "byte_size", "error_code",
			"group_id", "metadata", "worker_id", "queued_at", "started_at",
			"completed_at", "expires_at",
		}))
	got, err = repo.GetProcessingJob(ctx, "job-1", "user-2")
	if err != nil || got != nil {
		t.Fatalf("non-owner fetch = %+v, %v (want nil, nil)", got, err)
	}
}

func TestRequeueStaleProcessingJobs(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'queued'").
		WithArgs(5 * time.Minute).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	count, err := repo.RequeueStaleProcessingJobs(context.Background(), 5*time.Minute)
	if err != nil || count != 2 {
		t.Fatalf("requeued = %d, %v", count, err)
	}
}

func TestPurgeExpiredProcessingJobs(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectExec("DELETE FROM media_processing_jobs.*expires_at").
		WillReturnResult(pgxmock.NewResult("DELETE", 3))
	count, err := repo.PurgeExpiredProcessingJobs(context.Background())
	if err != nil || count != 3 {
		t.Fatalf("purged = %d, %v", count, err)
	}
}

func TestAbandonedQuarantineListsIncompleteJobs(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery("SELECT id, quarantine_key FROM media_processing_jobs").
		WithArgs(time.Hour).
		WillReturnRows(pgxmock.NewRows([]string{"id", "quarantine_key"}).
			AddRow("job-1", "quarantine/raw-1").
			AddRow("job-2", "quarantine/raw-2"))
	items, err := repo.AbandonedQuarantine(context.Background(), time.Hour)
	if err != nil || len(items) != 2 || items[0].JobID != "job-1" || items[1].QuarantineKey != "quarantine/raw-2" {
		t.Fatalf("abandoned = %+v, %v", items, err)
	}
}

func TestProcessingJobDTOOmitsSecrets(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	ready := sampleProcessingJob()
	ready.Status = models.MediaProcessingStatusReady
	ready.CompletedAt = ptrTime(now)
	dto := ready.ToResponse()
	result, ok := dto.Result.(*models.MediaProcessingResult)
	if !ok || result.ID != "photo-1" || dto.ErrorCode != "" {
		t.Fatalf("ready DTO = %+v", dto)
	}

	failed := &models.MediaProcessingJob{
		ID:            "job-2",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChat,
		Status:        models.MediaProcessingStatusFailed,
		QuarantineKey: "quarantine/secret",
		CanonicalKey:  "chat-media/secret",
		ErrorCode:     "transcode_failed",
		Metadata:      []byte(`{"secret":true}`),
		QueuedAt:      now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
	dto = failed.ToResponse()
	if dto.Result != nil || dto.ErrorCode != "transcode_failed" {
		t.Fatalf("failed DTO = %+v", dto)
	}

	queued := &models.MediaProcessingJob{
		ID:            "job-3",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChallenge,
		Status:        models.MediaProcessingStatusQueued,
		QuarantineKey: "quarantine/secret",
		Metadata:      []byte(`{"secret":true}`),
		QueuedAt:      now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
	dto = queued.ToResponse()
	if dto.Result != nil || dto.ErrorCode != "" {
		t.Fatalf("queued DTO = %+v", dto)
	}
}
