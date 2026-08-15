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
	"testing"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"

	"github.com/google/uuid"
)

// --- fakes -----------------------------------------------------------------

// scriptedRunner returns canned responses in invocation order, so one test can
// drive ffprobe and ffmpeg with distinct outcomes deterministically. When
// canonicalOutput is set, every successful ffmpeg invocation also writes those
// bytes to its output path (the final argument), mirroring what a real ffmpeg
// run produces so the worker can read the canonical file.
type scriptedRunner struct {
	responses       []runResponse
	invoked         []string
	idx             int
	canonicalOutput []byte
}

type runResponse struct {
	stdout   []byte
	exitCode int
	err      error
}

func (s *scriptedRunner) Run(_ context.Context, name string, args ...string) ([]byte, int, error) {
	if s.idx >= len(s.responses) {
		return nil, 1, errors.New("unexpected runner invocation")
	}
	s.invoked = append(s.invoked, name)
	r := s.responses[s.idx]
	s.idx++
	if name == "ffmpeg" && r.err == nil && r.exitCode == 0 && s.canonicalOutput != nil && len(args) > 0 {
		_ = os.WriteFile(args[len(args)-1], s.canonicalOutput, 0o600)
	}
	return r.stdout, r.exitCode, r.err
}

type challengeCompletion struct {
	jobID    string
	photos   []*models.Photo
	mimeType string
	byteSize int64
}

type chatCompletion struct {
	jobID string
	msg   *models.Message
	asset *models.ChatMedia
}

type deletionCall struct {
	source string
	keys   []string
}

// fakeJobStore is an in-memory JobStore recording every worker interaction.
type fakeJobStore struct {
	claimQueue           []*models.MediaProcessingJob
	claimedWorkerIDs     []string
	failures             map[string]string
	challengeCompletions []challengeCompletion
	chatCompletions      []chatCompletion
	deletions            []deletionCall
	completeErr          error
}

func (f *fakeJobStore) ClaimProcessingJob(_ context.Context, workerID string) (*models.MediaProcessingJob, error) {
	f.claimedWorkerIDs = append(f.claimedWorkerIDs, workerID)
	if len(f.claimQueue) == 0 {
		return nil, nil
	}
	job := f.claimQueue[0]
	f.claimQueue = f.claimQueue[1:]
	job.WorkerID = workerID
	return job, nil
}

func (f *fakeJobStore) FailClaimedProcessingJob(_ context.Context, jobID, _, quarantineKey, errorCode string) error {
	if f.failures == nil {
		f.failures = map[string]string{}
	}
	f.failures[jobID] = errorCode
	f.deletions = append(f.deletions, deletionCall{source: "media-processing", keys: []string{quarantineKey}})
	return nil
}

func (f *fakeJobStore) EnqueueMediaDeletion(_ context.Context, source string, keys []string) error {
	f.deletions = append(f.deletions, deletionCall{source: source, keys: append([]string(nil), keys...)})
	return nil
}

func (f *fakeJobStore) CompleteChallengeProcessing(_ context.Context, jobID, _, quarantineKey string, photos []*models.Photo, mimeType string, byteSize int64) error {
	if f.completeErr != nil {
		return f.completeErr
	}
	f.challengeCompletions = append(f.challengeCompletions, challengeCompletion{jobID: jobID, photos: photos, mimeType: mimeType, byteSize: byteSize})
	f.deletions = append(f.deletions, deletionCall{source: "media-processing", keys: []string{quarantineKey}})
	return nil
}

func (f *fakeJobStore) CompleteChatProcessing(_ context.Context, jobID, _, quarantineKey string, msg *models.Message, asset *models.ChatMedia) error {
	if f.completeErr != nil {
		return f.completeErr
	}
	// Mirror the real repository: on success the message carries the media
	// reference the worker broadcasts to clients.
	msg.Kind = "media"
	msg.MediaID = &asset.ID
	msg.MediaType = asset.MIMEType
	f.chatCompletions = append(f.chatCompletions, chatCompletion{jobID: jobID, msg: msg, asset: asset})
	f.deletions = append(f.deletions, deletionCall{source: "media-processing", keys: []string{quarantineKey}})
	return nil
}

// fakeObjectStore is an in-memory ObjectStore.
type fakeObjectStore struct {
	objects map[string][]byte
}

func (s *fakeObjectStore) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	data, err := io.ReadAll(io.LimitReader(body, size+1))
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fmt.Errorf("stored %d bytes, expected %d", len(data), size)
	}
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = data
	return nil
}

func (s *fakeObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeObjectStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *fakeObjectStore) Stat(_ context.Context, key string) (int64, error) {
	data, ok := s.objects[key]
	if !ok {
		return 0, storage.ErrObjectNotFound
	}
	return int64(len(data)), nil
}

func (s *fakeObjectStore) Health(context.Context) error { return nil }

// fakeBroadcaster records realtime fan-out.
type fakeBroadcaster struct {
	broadcast []models.Message
	persisted []models.Message
}

func (b *fakeBroadcaster) Broadcast(m models.Message) { b.broadcast = append(b.broadcast, m) }
func (b *fakeBroadcaster) BroadcastPersisted(m models.Message) {
	b.persisted = append(b.persisted, m)
}

// fakeNotifier records Web Push challenge notifications.
type fakeNotifier struct {
	notified []string // "groupID:photoID"
}

func (n *fakeNotifier) NotifyNewChallenge(_ context.Context, groupID, _ string, photoID string) {
	n.notified = append(n.notified, groupID+":"+photoID)
}

// --- fixtures --------------------------------------------------------------

var fixedNow = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

func newTestWorker(jobs *fakeJobStore, store *fakeObjectStore, runner CommandRunner, broadcast Broadcaster, notify PushNotifier) *Worker {
	return NewWorker(WorkerDeps{
		Jobs:           jobs,
		Store:          store,
		Broadcaster:    broadcast,
		Notifier:       notify,
		Runner:         runner,
		MaxInputBytes:  10 << 20,
		ChallengeTTL:   24 * time.Hour,
		PhotoRetention: 720 * time.Hour,
		WorkerID:       "test-worker",
		PollInterval:   time.Millisecond,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:            func() time.Time { return fixedNow },
	})
}

func validProbeJSON() []byte {
	return []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720,"duration":"10.0","avg_frame_rate":"30/1"}],"format":{"duration":"10.0"}}`)
}

// canonicalBytes is what a successful (fake) ffmpeg run writes to its output
// path. Every byte-size assertion derives from it.
var canonicalBytes = []byte("canonical video bytes")

func challengeJob(md models.ChallengeProcessingMetadata) *models.MediaProcessingJob {
	raw, _ := json.Marshal(md)
	return &models.MediaProcessingJob{
		ID:            "job-1",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChallenge,
		Status:        models.MediaProcessingStatusProcessing,
		QuarantineKey: "quarantine/job-1",
		GroupID:       "g1",
		Metadata:      raw,
		QueuedAt:      fixedNow.Add(-time.Minute),
		ExpiresAt:     fixedNow.Add(24 * time.Hour),
	}
}

func chatJob(md models.ChatProcessingMetadata) *models.MediaProcessingJob {
	raw, _ := json.Marshal(md)
	return &models.MediaProcessingJob{
		ID:            "job-2",
		UserID:        "user-1",
		Kind:          models.MediaProcessingKindChat,
		Status:        models.MediaProcessingStatusProcessing,
		QuarantineKey: "quarantine/job-2",
		GroupID:       "g1",
		Metadata:      raw,
		QueuedAt:      fixedNow.Add(-time.Minute),
		ExpiresAt:     fixedNow.Add(24 * time.Hour),
	}
}

func deletionsContainKey(deletions []deletionCall, key string) bool {
	for _, d := range deletions {
		for _, k := range d.keys {
			if k == key {
				return true
			}
		}
	}
	return false
}

func rawQuarantineKey(t *testing.T, job *models.MediaProcessingJob) string {
	t.Helper()
	if !storage.IsQuarantineKey(job.QuarantineKey) {
		t.Fatalf("job quarantine key %q is not under the private prefix", job.QuarantineKey)
	}
	return job.QuarantineKey
}

// --- tests -----------------------------------------------------------------

func TestProcessChallengeHappyPath(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	broadcast := &fakeBroadcaster{}
	notify := &fakeNotifier{}
	runner := &scriptedRunner{canonicalOutput: canonicalBytes, responses: []runResponse{
		{stdout: validProbeJSON()}, // ffprobe
		{exitCode: 0},              // ffmpeg
	}}
	worker := newTestWorker(jobs, store, runner, broadcast, notify)

	job := challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1", "g2"}, Lat: 1.5, Long: 2.5, HideLocation: true})
	worker.process(context.Background(), job)

	if len(jobs.failures) != 0 {
		t.Fatalf("unexpected failures: %v", jobs.failures)
	}
	if len(jobs.challengeCompletions) != 1 {
		t.Fatalf("challenge completions = %d, want 1", len(jobs.challengeCompletions))
	}
	completed := jobs.challengeCompletions[0]
	if completed.jobID != "job-1" || completed.mimeType != "video/mp4" || completed.byteSize != int64(len(canonicalBytes)) {
		t.Fatalf("completion = %+v", completed)
	}
	if len(completed.photos) != 2 {
		t.Fatalf("photos = %d, want 2", len(completed.photos))
	}
	for i, photo := range completed.photos {
		if photo.UserID != "user-1" || photo.LifecycleStatus != "ready" || photo.HideLocation != true {
			t.Fatalf("photo %d = %+v", i, photo)
		}
		if photo.MIMEType != "video/mp4" || photo.ByteSize != int64(len(canonicalBytes)) {
			t.Fatalf("photo %d media = %+v", i, photo)
		}
		if !storage.IsCanonicalKey(photo.StorageKey) {
			t.Fatalf("photo %d storage key %q is not canonical", i, photo.StorageKey)
		}
		if _, err := uuid.Parse(photo.ID); err != nil {
			t.Fatalf("photo %d id %q is not a UUID", i, photo.ID)
		}
		if photo.ExpiresAt != fixedNow.Add(24*time.Hour) || photo.RetentionAt != fixedNow.Add(720*time.Hour) {
			t.Fatalf("photo %d timestamps = expires %v retention %v", i, photo.ExpiresAt, photo.RetentionAt)
		}
	}
	// One broadcast + one push per target group, each carrying a photo id.
	if len(broadcast.broadcast) != 2 || len(notify.notified) != 2 {
		t.Fatalf("broadcasts = %d, notified = %d (want 2, 2)", len(broadcast.broadcast), len(notify.notified))
	}
	for i, msg := range broadcast.broadcast {
		if msg.Kind != "challenge" || msg.GroupID != completed.photos[i].GroupID || msg.PhotoID == nil || msg.Content != "" {
			t.Fatalf("broadcast %d = %+v", i, msg)
		}
	}
	if !deletionsContainKey(jobs.deletions, rawQuarantineKey(t, job)) {
		t.Fatalf("quarantine source was not enqueued for deletion: %+v", jobs.deletions)
	}
	if len(jobs.chatCompletions) != 0 {
		t.Fatalf("unexpected chat completions: %+v", jobs.chatCompletions)
	}
}

func TestProcessChatHappyPath(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-2": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	broadcast := &fakeBroadcaster{}
	notify := &fakeNotifier{}
	runner := &scriptedRunner{canonicalOutput: canonicalBytes, responses: []runResponse{
		{stdout: validProbeJSON()}, // ffprobe
		{exitCode: 0},              // ffmpeg
	}}
	worker := newTestWorker(jobs, store, runner, broadcast, notify)

	replyTo := "msg-0"
	job := chatJob(models.ChatProcessingMetadata{GroupID: "g1", Content: "check this", ReplyToID: &replyTo})
	worker.process(context.Background(), job)

	if len(jobs.failures) != 0 {
		t.Fatalf("unexpected failures: %v", jobs.failures)
	}
	if len(jobs.chatCompletions) != 1 {
		t.Fatalf("chat completions = %d, want 1", len(jobs.chatCompletions))
	}
	completed := jobs.chatCompletions[0]
	if completed.jobID != "job-2" || completed.msg.ID == "" || completed.asset.ID == "" {
		t.Fatalf("completion = %+v", completed)
	}
	if completed.msg.Kind != "media" || completed.msg.MediaID == nil || *completed.msg.MediaID != completed.asset.ID {
		t.Fatalf("message media reference not set: %+v", completed.msg)
	}
	if !storage.IsCanonicalKey(completed.asset.StorageKey) {
		t.Fatalf("asset storage key %q is not canonical", completed.asset.StorageKey)
	}
	if len(broadcast.persisted) != 1 || broadcast.persisted[0].ID != completed.msg.ID {
		t.Fatalf("persisted broadcasts = %+v, want exactly one", broadcast.persisted)
	}
	if len(broadcast.broadcast) != 0 {
		t.Fatalf("unexpected live broadcasts: %+v", broadcast.broadcast)
	}
	if len(notify.notified) != 0 {
		t.Fatalf("unexpected push notifications: %v", notify.notified)
	}
	if !deletionsContainKey(jobs.deletions, rawQuarantineKey(t, job)) {
		t.Fatalf("quarantine source was not enqueued for deletion: %+v", jobs.deletions)
	}
}
