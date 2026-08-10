package mediaprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/google/uuid"
)

// JobStore is the persistence seam the worker needs to claim, fail, and
// complete processing jobs and to enqueue durable deletion of raw sources and
// partial canonical objects. *repository.Repository satisfies it.
type JobStore interface {
	ClaimProcessingJob(ctx context.Context, workerID string) (*models.MediaProcessingJob, error)
	FailProcessingJob(ctx context.Context, jobID, errorCode string) error
	EnqueueMediaDeletion(ctx context.Context, source string, keys []string) error
	CompleteChallengeProcessing(ctx context.Context, jobID string, photos []*models.Photo, mimeType string, byteSize int64) error
	CompleteChatProcessing(ctx context.Context, jobID string, msg *models.Message, asset *models.ChatMedia) error
}

// Broadcaster is the realtime fan-out the worker uses to announce a freshly
// completed challenge or chat message exactly once after its transaction has
// committed. *chat.Hub satisfies it.
type Broadcaster interface {
	Broadcast(message models.Message)
	BroadcastPersisted(message models.Message)
}

// PushNotifier fans a Web Push notification for a freshly resolved challenge.
// *push.Service satisfies it.
type PushNotifier interface {
	NotifyNewChallenge(ctx context.Context, groupID, excludeUserID, photoID string)
}

// WorkerDeps wires the media-processing worker. Jobs, Store, Broadcaster, and
// Notifier are the only external seams; Runner is the ffprobe/ffmpeg seam
// (nil selects the OS runner). The worker runs as a single in-process
// goroutine inside the server process (see main.go), sharing the realtime hub
// and push notifier so a completed record is announced exactly once.
type WorkerDeps struct {
	Jobs           JobStore
	Store          storage.ObjectStore
	Broadcaster    Broadcaster
	Notifier       PushNotifier
	Runner         CommandRunner
	MaxInputBytes  int64
	ChallengeTTL   time.Duration
	PhotoRetention time.Duration
	WorkerID       string
	PollInterval   time.Duration
	Logger         *slog.Logger
	Now            func() time.Time
}

// Worker claims quarantined video processing jobs and promotes them into
// canonical media records. It is deliberately a single replica: the product
// runs one backend replica, and two workers over the same database would each
// broadcast the other's completed records. The claim is still atomic
// (FOR UPDATE SKIP LOCKED) so a restarting worker never collides with itself.
type Worker struct {
	jobs           JobStore
	store          storage.ObjectStore
	broadcast      Broadcaster
	notify         PushNotifier
	runner         CommandRunner
	maxInputBytes  int64
	challengeTTL   time.Duration
	photoRetention time.Duration
	workerID       string
	pollInterval   time.Duration
	logger         *slog.Logger
	now            func() time.Time
}

const (
	// jobTimeout bounds a single job's validate+transcode work to 60 seconds
	// (F-10 acceptance criterion). The child rlimits and the worker container's
	// cpus/mem/pids bounds back this deadline up.
	jobTimeout = 60 * time.Second
	// defaultMaxInputBytes is the 10 MiB input cap applied when no explicit
	// configuration is injected (it mirrors UPLOAD_MAX_BYTES).
	defaultMaxInputBytes = 10 << 20
	// canonicalMIMEType is the only media type the worker ever produces.
	canonicalMIMEType = "video/mp4"
	// deletionSource identifies quarantine-source deletion jobs enqueued by the
	// worker. It is informational (observability only).
	deletionSource = "media-processing"
)

// NewWorker builds a worker from its explicit dependencies. A nil Runner
// selects the OS command runner, and a non-positive PollInterval falls back to
// one second.
func NewWorker(deps WorkerDeps) *Worker {
	interval := deps.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	maxBytes := deps.MaxInputBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxInputBytes
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Worker{
		jobs:           deps.Jobs,
		store:          deps.Store,
		broadcast:      deps.Broadcaster,
		notify:         deps.Notifier,
		runner:         deps.Runner,
		maxInputBytes:  maxBytes,
		challengeTTL:   deps.ChallengeTTL,
		photoRetention: deps.PhotoRetention,
		workerID:       deps.WorkerID,
		pollInterval:   interval,
		logger:         deps.Logger,
		now:            now,
	}
}

// Run claims and processes jobs until ctx is canceled. It is intended to run
// as a single goroutine started by the composition root (main.go).
func (w *Worker) Run(ctx context.Context) {
	for {
		if w.RunOnce(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.pollInterval):
		}
	}
}

// RunOnce claims at most one job and processes it. It returns true when a job
// was processed so Run can immediately claim the next one. Tests use it for
// deterministic single-job execution.
func (w *Worker) RunOnce(ctx context.Context) bool {
	job, err := w.jobs.ClaimProcessingJob(ctx, w.workerID)
	if err != nil {
		w.logError("claiming media processing job failed", "error", err)
		return false
	}
	if job == nil {
		return false
	}
	w.process(ctx, job)
	return true
}

// process handles one claimed job end to end: download the quarantine object,
// validate and transcode it under the 60-second per-job bound, write the
// canonical object(s), create the challenge/chat record and complete the job
// atomically, then announce it exactly once and enqueue durable deletion of
// the raw source. Every failure path marks the job failed with a stable code
// and enqueues durable cleanup of the source and any partial canonical
// objects.
func (w *Worker) process(ctx context.Context, job *models.MediaProcessingJob) {
	w.logInfo("media processing job claimed", "job_id", job.ID, "kind", job.Kind, "worker", w.workerID)

	dir, err := os.MkdirTemp("", "geoguessme-media-*")
	if err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	defer os.RemoveAll(dir)

	sourcePath := filepath.Join(dir, "source.bin")
	if err := w.download(ctx, job.QuarantineKey, sourcePath); err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}

	// The 60-second deadline covers probe + transcode only; the success and
	// cleanup paths below use the outer context so a deadline expiry can never
	// orphan the quarantine object.
	ctxJob, cancel := context.WithTimeout(ctx, jobTimeout)
	spec, err := Validate(ctxJob, sourcePath, w.maxInputBytes, w.runner)
	if err != nil {
		cancel()
		w.fail(ctx, job, processingErrorCode(err), err)
		return
	}
	canonicalPath := filepath.Join(dir, "canonical.mp4")
	if err := Transcode(ctxJob, sourcePath, canonicalPath, spec.HasAudio, w.runner); err != nil {
		cancel()
		w.fail(ctx, job, processingErrorCode(err), err)
		return
	}
	cancel()

	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}

	now := w.now().UTC()
	switch job.Kind {
	case models.MediaProcessingKindChallenge:
		w.finishChallenge(ctx, job, data, now)
	case models.MediaProcessingKindChat:
		w.finishChat(ctx, job, data, now)
	default:
		w.fail(ctx, job, ErrorTranscodeFailed, fmt.Errorf("unknown media processing kind %q", job.Kind))
	}
}

// finishChallenge writes one canonical object per target group, creates the
// per-group challenge photos and completes the job in a single transaction,
// then broadcasts and notifies once per photo.
func (w *Worker) finishChallenge(ctx context.Context, job *models.MediaProcessingJob, data []byte, now time.Time) {
	var md models.ChallengeProcessingMetadata
	if err := json.Unmarshal(job.Metadata, &md); err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	if len(md.GroupIDs) == 0 {
		w.fail(ctx, job, ErrorTranscodeFailed, errors.New("challenge metadata carries no target groups"))
		return
	}
	photos := make([]*models.Photo, 0, len(md.GroupIDs))
	keys := make([]string, 0, len(md.GroupIDs))
	for _, groupID := range md.GroupIDs {
		key, err := storage.CanonicalKey("photo", uuid.NewString())
		if err != nil {
			w.cleanupKeys(ctx, keys)
			w.fail(ctx, job, ErrorTranscodeFailed, err)
			return
		}
		if err := w.store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), canonicalMIMEType); err != nil {
			w.cleanupKeys(ctx, keys)
			w.fail(ctx, job, ErrorTranscodeFailed, err)
			return
		}
		keys = append(keys, key)
		photos = append(photos, &models.Photo{
			ID:              uuid.NewString(),
			UserID:          job.UserID,
			GroupID:         groupID,
			StorageKey:      key,
			MIMEType:        canonicalMIMEType,
			ByteSize:        int64(len(data)),
			Lat:             md.Lat,
			Long:            md.Long,
			LifecycleStatus: "ready",
			HideLocation:    md.HideLocation,
			CreatedAt:       now,
			ExpiresAt:       now.Add(w.challengeTTL),
			RetentionAt:     now.Add(w.photoRetention),
		})
	}
	if err := w.jobs.CompleteChallengeProcessing(ctx, job.ID, photos, canonicalMIMEType, int64(len(data))); err != nil {
		w.cleanupKeys(ctx, keys)
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	// The transaction committed; only now announce the challenges, once each.
	for _, photo := range photos {
		photoID := photo.ID
		message := models.Message{
			ID:        uuid.NewString(),
			GroupID:   photo.GroupID,
			UserID:    job.UserID,
			Kind:      "challenge",
			PhotoID:   &photoID,
			Content:   "",
			CreatedAt: now,
		}
		if w.broadcast != nil {
			w.broadcast.Broadcast(message)
		}
		if w.notify != nil {
			w.notify.NotifyNewChallenge(ctx, photo.GroupID, job.UserID, photo.ID)
		}
	}
	w.enqueueSourceDeletion(ctx, job)
}

// finishChat writes one canonical object, creates the chat message and its
// attachment and completes the job in a single transaction, then broadcasts
// the persisted message exactly once.
func (w *Worker) finishChat(ctx context.Context, job *models.MediaProcessingJob, data []byte, now time.Time) {
	var md models.ChatProcessingMetadata
	if err := json.Unmarshal(job.Metadata, &md); err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	key, err := storage.CanonicalKey("chat", uuid.NewString())
	if err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	if err := w.store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), canonicalMIMEType); err != nil {
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	asset := &models.ChatMedia{
		ID:         uuid.NewString(),
		GroupID:    md.GroupID,
		UserID:     job.UserID,
		StorageKey: key,
		MIMEType:   canonicalMIMEType,
		ByteSize:   int64(len(data)),
		CreatedAt:  now,
	}
	msg := &models.Message{
		ID:        uuid.NewString(),
		GroupID:   md.GroupID,
		UserID:    job.UserID,
		Kind:      "media",
		ReplyToID: md.ReplyToID,
		Content:   md.Content,
		CreatedAt: now,
	}
	if err := w.jobs.CompleteChatProcessing(ctx, job.ID, msg, asset); err != nil {
		w.cleanupKeys(ctx, []string{key})
		w.fail(ctx, job, ErrorTranscodeFailed, err)
		return
	}
	if w.broadcast != nil {
		w.broadcast.BroadcastPersisted(*msg)
	}
	w.enqueueSourceDeletion(ctx, job)
}

// download streams a quarantine object into a local temp file.
func (w *Worker) download(ctx context.Context, key, dstPath string) error {
	object, err := w.store.Get(ctx, key)
	if err != nil {
		return err
	}
	defer object.Close()
	file, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, object)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// fail marks the job failed with a stable code and enqueues durable deletion
// of the raw quarantine source. Canonical objects written earlier are cleaned
// by the caller before fail is reached.
func (w *Worker) fail(ctx context.Context, job *models.MediaProcessingJob, code string, cause error) {
	w.logError("media processing job failed", "job_id", job.ID, "code", code, "error", cause)
	if err := w.jobs.FailProcessingJob(ctx, job.ID, code); err != nil {
		w.logError("marking media processing job failed failed", "job_id", job.ID, "error", err)
	}
	if err := w.jobs.EnqueueMediaDeletion(ctx, deletionSource, []string{job.QuarantineKey}); err != nil {
		w.logError("enqueuing quarantine deletion failed", "job_id", job.ID, "error", err)
	}
}

// cleanupKeys best-effort deletes written canonical objects after a failure;
// objects that refuse immediate deletion become durable deletion jobs so no
// bytes are ever orphaned.
func (w *Worker) cleanupKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		if err := w.store.Delete(ctx, key); err != nil {
			if enqueueErr := w.jobs.EnqueueMediaDeletion(ctx, "media-processing-cleanup", []string{key}); enqueueErr != nil {
				w.logError("persisting canonical cleanup failed", "storage_key", key, "error", enqueueErr)
			}
		}
	}
}

// enqueueSourceDeletion records the raw quarantine source for durable
// deletion after a successful promotion.
func (w *Worker) enqueueSourceDeletion(ctx context.Context, job *models.MediaProcessingJob) {
	if err := w.jobs.EnqueueMediaDeletion(ctx, deletionSource, []string{job.QuarantineKey}); err != nil {
		w.logError("enqueuing quarantine deletion failed", "job_id", job.ID, "error", err)
	}
}

// processingErrorCode maps a validation or transcode error to its stable code,
// falling back to a generic transcode failure for infrastructure errors so an
// I/O problem is never misreported as an input rejection.
func processingErrorCode(err error) string {
	if code := ErrorCode(err); code != "" {
		return code
	}
	return ErrorTranscodeFailed
}

func (w *Worker) logInfo(message string, args ...any) {
	if w.logger != nil {
		w.logger.Info(message, args...)
	}
}

func (w *Worker) logError(message string, args ...any) {
	if w.logger != nil {
		w.logger.Warn(message, args...)
	}
}
