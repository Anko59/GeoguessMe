package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

// testWebM is the smallest valid browser-recorded WebM container signature the
// server's container-byte detection recognizes as a video.
var testWebM = []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}

// processingJobRows builds the row produced by the job SELECTs/RETURNING.
// pgxmock scans positionally, so the column names are only for readability.
func processingJobRows(job *models.MediaProcessingJob) *pgxmock.Rows {
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

func sampleQueuedJob(now time.Time) *models.MediaProcessingJob {
	return &models.MediaProcessingJob{
		ID:            "00000000-0000-0000-0000-000000000010",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChallenge,
		Status:        models.MediaProcessingStatusQueued,
		QuarantineKey: "quarantine/00000000-0000-0000-0000-000000000010",
		MIMEType:      "video/webm",
		ByteSize:      int64(len(testWebM)),
		GroupID:       "00000000-0000-0000-0000-000000000001",
		Metadata:      []byte(`{"secret":true}`),
		QueuedAt:      now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
}

// TestUploadChatVideoQueuesProcessingJobAndQuarantines proves the chat video
// branch answers 202, quarantines the raw source, and creates no visible
// message row (the only database write is the queued processing job).
func TestUploadChatVideoQueuesProcessingJobAndQuarantines(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	chatAPI := newChatAPI(t, mock, store, hub)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO media_processing_jobs").WithArgs(pgxmock.AnyArg(), "user-1", models.MediaProcessingKindChat, pgxmock.AnyArg(), groupID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	request := multipartChatMediaUpload(t, groupID, "capture.webm", testWebM)
	recorder := httptest.NewRecorder()
	chatAPI.UploadChatMedia(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("chat video upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var job models.MediaProcessingJobResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Status != models.MediaProcessingStatusQueued || job.Kind != models.MediaProcessingKindChat {
		t.Fatalf("job DTO = %+v", job)
	}
	raw, err := store.Get(context.Background(), storage.QuarantineKey(job.ID))
	if err != nil {
		t.Fatalf("chat quarantine object missing: %v", err)
	}
	defer raw.Close()
	if got, _ := io.ReadAll(raw); !bytes.Equal(got, testWebM) {
		t.Fatalf("chat quarantine bytes differ: got %d bytes, want %d", len(got), len(testWebM))
	}
}

// recordingDeleteStore wraps a real object store and records every Delete so
// tests can assert compensation deletes happened for the right key.
type recordingDeleteStore struct {
	storage.ObjectStore
	deleted []string
}

func (s *recordingDeleteStore) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.ObjectStore.Delete(ctx, key)
}

// TestVideoUploadJobInsertFailureCompensatesQuarantine proves a storage put
// followed by a failed job insert deletes the quarantine object so no raw
// bytes are left unowned.
func TestVideoUploadJobInsertFailureCompensatesQuarantine(t *testing.T) {
	local, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingDeleteStore{ObjectStore: local}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO media_processing_jobs").WithArgs(pgxmock.AnyArg(), "user-1", models.MediaProcessingKindChallenge, pgxmock.AnyArg(), groupID, pgxmock.AnyArg()).WillReturnError(errors.New("db unavailable"))
	request, err := multipartMediaUpload(t, groupID, "capture.webm", testWebM)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("video upload status = %d (%s), want 500", recorder.Code, recorder.Body.String())
	}
	if len(store.deleted) != 1 || !strings.HasPrefix(store.deleted[0], storage.QuarantineKeyPrefix) {
		t.Fatalf("compensation deletes = %v, want exactly one quarantine key", store.deleted)
	}
}

// TestChatVideoUploadJobInsertFailureCompensatesQuarantine proves the chat
// video branch compensates the quarantine object the same way when the job
// insert fails.
func TestChatVideoUploadJobInsertFailureCompensatesQuarantine(t *testing.T) {
	local, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingDeleteStore{ObjectStore: local}
	mock := newMockPool(t)
	hub := chat.NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)
	chatAPI := newChatAPI(t, mock, store, hub)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO media_processing_jobs").WithArgs(pgxmock.AnyArg(), "user-1", models.MediaProcessingKindChat, pgxmock.AnyArg(), groupID, pgxmock.AnyArg()).WillReturnError(errors.New("db unavailable"))
	recorder := httptest.NewRecorder()
	chatAPI.UploadChatMedia(recorder, multipartChatMediaUpload(t, groupID, "capture.webm", testWebM))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("chat video upload status = %d (%s), want 500", recorder.Code, recorder.Body.String())
	}
	if len(store.deleted) != 1 || !strings.HasPrefix(store.deleted[0], storage.QuarantineKeyPrefix) {
		t.Fatalf("chat compensation deletes = %v, want exactly one quarantine key", store.deleted)
	}
}

// TestGetMediaProcessingJobStates covers the owner-only status endpoint across
// every lifecycle state, including the ready result enrichment and the
// non-sensitive failed error code, and proves storage keys never serialize.
func TestGetMediaProcessingJobStates(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	jobID := "00000000-0000-0000-0000-000000000010"
	groupID := "00000000-0000-0000-0000-000000000001"

	t.Run("queued", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		job := sampleQueuedJob(now)
		mock.ExpectQuery("SELECT id, user_id, kind").WithArgs(jobID, "user-1").WillReturnRows(processingJobRows(job))
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodGet, "/", "", "user-1")
		req.SetPathValue("jobID", jobID)
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		if !strings.Contains(body, `"status":"queued"`) || strings.Contains(body, "quarantine") || strings.Contains(body, `"secret"`) {
			t.Fatalf("queued body leaked internals or has wrong status: %s", body)
		}
	})

	t.Run("ready challenge result", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		job := sampleQueuedJob(now)
		job.Status = models.MediaProcessingStatusReady
		job.ResultKind = "photo"
		job.ResultID = "00000000-0000-0000-0000-000000000011"
		job.CanonicalKey = "photos/canonical"
		job.CompletedAt = &now
		mock.ExpectQuery("SELECT id, user_id, kind").WithArgs(jobID, "user-1").WillReturnRows(processingJobRows(job))
		photo := &models.Photo{ID: job.ResultID, UserID: "user-1", GroupID: groupID, StorageKey: "photos/canonical", MIMEType: "video/mp4", ByteSize: 12, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(job.ResultID).WillReturnRows(handlerPhotoRows(photo))
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodGet, "/", "", "user-1")
		req.SetPathValue("jobID", jobID)
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
		}
		var got struct {
			Status string `json:"status"`
			Result struct {
				ID      string `json:"id"`
				GroupID string `json:"group_id"`
			} `json:"result"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != string(models.MediaProcessingStatusReady) || got.Result.ID != job.ResultID || got.Result.GroupID != groupID {
			t.Fatalf("ready body = %+v", got)
		}
		if got.ErrorCode != "" {
			t.Fatalf("ready job leaked error code %q", got.ErrorCode)
		}
		body := recorder.Body.String()
		if strings.Contains(body, "quarantine") || strings.Contains(body, "canonical") || strings.Contains(body, `"secret"`) {
			t.Fatalf("ready body leaked storage keys or metadata: %s", body)
		}
	})

	t.Run("failed exposes stable error code", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		job := sampleQueuedJob(now)
		job.Status = models.MediaProcessingStatusFailed
		job.ErrorCode = "transcode_failed"
		job.CompletedAt = &now
		mock.ExpectQuery("SELECT id, user_id, kind").WithArgs(jobID, "user-1").WillReturnRows(processingJobRows(job))
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodGet, "/", "", "user-1")
		req.SetPathValue("jobID", jobID)
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
		}
		var got struct {
			Status    string `json:"status"`
			Result    any    `json:"result"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != string(models.MediaProcessingStatusFailed) || got.ErrorCode != "transcode_failed" || got.Result != nil {
			t.Fatalf("failed body = %+v", got)
		}
	})

	t.Run("non-owner and unknown job are a uniform 404", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectQuery("SELECT id, user_id, kind").WithArgs(jobID, "user-2").WillReturnError(pgx.ErrNoRows)
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodGet, "/", "", "user-2")
		req.SetPathValue("jobID", jobID)
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d (%s), want 404", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("invalid job id is rejected", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodGet, "/", "", "user-1")
		req.SetPathValue("jobID", "not-a-uuid")
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d (%s), want 400", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		recorder := httptest.NewRecorder()
		req := requestWithUser(http.MethodDelete, "/", "", "user-1")
		req.SetPathValue("jobID", jobID)
		gameAPI.GetMediaProcessingJob(recorder, req)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d (%s), want 405", recorder.Code, recorder.Body.String())
		}
	})
}
