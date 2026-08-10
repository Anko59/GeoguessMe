package mediaprocessing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"geoguessme/internal/models"
	"geoguessme/internal/storage"
)

func TestProcessRejectsMultiStreamVideo(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	runner := &scriptedRunner{responses: []runResponse{
		{stdout: []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720,"duration":"10.0","avg_frame_rate":"30/1"},{"codec_type":"video","codec_name":"h264","width":640,"height":480,"duration":"10.0","avg_frame_rate":"30/1"}],"format":{"duration":"10.0"}}`)},
	}}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorMultiStream {
		t.Fatalf("failures = %v, want multi_stream", jobs.failures)
	}
	if len(jobs.challengeCompletions) != 0 || len(jobs.chatCompletions) != 0 {
		t.Fatalf("unexpected completions: %+v %+v", jobs.challengeCompletions, jobs.chatCompletions)
	}
	if len(store.objects) != 1 {
		t.Fatalf("no canonical object may be written, store has %v", store.objects)
	}
	if !deletionsContainKey(jobs.deletions, "quarantine/job-1") {
		t.Fatalf("raw source not enqueued for deletion: %+v", jobs.deletions)
	}
}

func TestProcessRejectsUnsupportedCodec(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	runner := &scriptedRunner{responses: []runResponse{
		{stdout: []byte(`{"streams":[{"codec_type":"video","codec_name":"av1","width":1280,"height":720,"duration":"10.0","avg_frame_rate":"30/1"}],"format":{"duration":"10.0"}}`)},
	}}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorUnsupportedCodec {
		t.Fatalf("failures = %v, want unsupported_codec", jobs.failures)
	}
}

func TestProcessRejectsTooLongVideo(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	runner := &scriptedRunner{responses: []runResponse{
		{stdout: []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720,"duration":"60.0","avg_frame_rate":"30/1"}],"format":{"duration":"60.0"}}`)},
	}}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorTooLong {
		t.Fatalf("failures = %v, want too_long", jobs.failures)
	}
}

func TestProcessTranscodeFailure(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	runner := &scriptedRunner{responses: []runResponse{
		{stdout: validProbeJSON()},
		{exitCode: 1}, // ffmpeg fails
	}}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorTranscodeFailed {
		t.Fatalf("failures = %v, want transcode_failed", jobs.failures)
	}
	if len(jobs.challengeCompletions) != 0 {
		t.Fatalf("unexpected completions: %+v", jobs.challengeCompletions)
	}
	if len(store.objects) != 1 {
		t.Fatalf("no canonical object may remain, store has %v", store.objects)
	}
	if !deletionsContainKey(jobs.deletions, "quarantine/job-1") {
		t.Fatalf("raw source not enqueued for deletion: %+v", jobs.deletions)
	}
}

func TestProcessTranscodeTimeout(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{}
	runner := &scriptedRunner{responses: []runResponse{
		{stdout: validProbeJSON()},
		{err: context.DeadlineExceeded}, // ffmpeg exceeds the 60s bound
	}}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorTimeout {
		t.Fatalf("failures = %v, want timeout", jobs.failures)
	}
	if !deletionsContainKey(jobs.deletions, "quarantine/job-1") {
		t.Fatalf("raw source not enqueued for deletion: %+v", jobs.deletions)
	}
}

func TestProcessMissingQuarantineSource(t *testing.T) {
	jobs := &fakeJobStore{}
	store := &fakeObjectStore{} // no quarantine object
	runner := &scriptedRunner{}
	worker := newTestWorker(jobs, store, runner, &fakeBroadcaster{}, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}}))

	if jobs.failures["job-1"] != ErrorTranscodeFailed {
		t.Fatalf("failures = %v, want generic transcode_failed", jobs.failures)
	}
	if len(runner.invoked) != 0 {
		t.Fatalf("ffprobe must not run without a source, invoked %v", runner.invoked)
	}
}

func TestRunOnceClaimsNothing(t *testing.T) {
	jobs := &fakeJobStore{}
	worker := newTestWorker(jobs, &fakeObjectStore{}, &scriptedRunner{}, &fakeBroadcaster{}, &fakeNotifier{})
	if worker.RunOnce(context.Background()) {
		t.Fatal("RunOnce claimed a job despite an empty queue")
	}
	if len(jobs.claimedWorkerIDs) != 1 || jobs.claimedWorkerIDs[0] != "test-worker" {
		t.Fatalf("claims = %v", jobs.claimedWorkerIDs)
	}
}

func TestProcessCompleteFailureCleansUpCanonicalObjects(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{completeErr: errors.New("database unavailable")}
	broadcast := &fakeBroadcaster{}
	runner := &scriptedRunner{canonicalOutput: canonicalBytes, responses: []runResponse{
		{stdout: validProbeJSON()},
		{exitCode: 0},
	}}
	worker := newTestWorker(jobs, store, runner, broadcast, &fakeNotifier{})

	worker.process(context.Background(), challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1", "g2"}}))

	if jobs.failures["job-1"] != ErrorTranscodeFailed {
		t.Fatalf("failures = %v, want transcode_failed", jobs.failures)
	}
	if len(broadcast.broadcast) != 0 {
		t.Fatalf("nothing may be broadcast after a failed completion: %+v", broadcast.broadcast)
	}
	// Both canonical objects written before the failed transaction must be gone.
	for key := range store.objects {
		if storage.IsCanonicalKey(key) {
			t.Fatalf("canonical object %q leaked after failure", key)
		}
	}
	if !deletionsContainKey(jobs.deletions, "quarantine/job-1") {
		t.Fatalf("raw source not enqueued for deletion: %+v", jobs.deletions)
	}
}

// TestRunOnceRecoversRequeuedJob proves the stale-requeue recovery path end to
// end: a job the cleanup runner returned to the queue is claimed again and
// reprocessed to completion (the claim is state-agnostic; RequeueStale in the
// cleanup runner moves it back).
func TestRunOnceRecoversRequeuedJob(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"quarantine/job-1": []byte("raw video bytes")}}
	jobs := &fakeJobStore{claimQueue: []*models.MediaProcessingJob{challengeJob(models.ChallengeProcessingMetadata{GroupIDs: []string{"g1"}})}}
	broadcast := &fakeBroadcaster{}
	runner := &scriptedRunner{canonicalOutput: canonicalBytes, responses: []runResponse{
		{stdout: validProbeJSON()},
		{exitCode: 0},
	}}
	worker := newTestWorker(jobs, store, runner, broadcast, &fakeNotifier{})

	if !worker.RunOnce(context.Background()) {
		t.Fatal("RunOnce did not process the claimed job")
	}
	if len(jobs.challengeCompletions) != 1 {
		t.Fatalf("reprocessed job not completed: %+v", jobs.challengeCompletions)
	}
	if len(broadcast.broadcast) != 1 {
		t.Fatalf("reprocessed job not announced: %+v", broadcast.broadcast)
	}
	if len(jobs.failures) != 0 {
		t.Fatalf("unexpected failures: %v", jobs.failures)
	}
}

func TestProcessingErrorCodeFallback(t *testing.T) {
	if got := processingErrorCode(validationError(ErrorTooLarge, "too big")); got != ErrorTooLarge {
		t.Fatalf("validation code = %q", got)
	}
	if got := processingErrorCode(errors.New("i/o timeout")); got != ErrorTranscodeFailed {
		t.Fatalf("fallback code = %q, want transcode_failed", got)
	}
}

func TestCanonicalMIMETypeAndKeys(t *testing.T) {
	key, err := storage.CanonicalKey("photo", "abc")
	if err != nil || !strings.HasPrefix(key, "photos/") {
		t.Fatalf("photo canonical key = %q, %v", key, err)
	}
	key, err = storage.CanonicalKey("chat", "abc")
	if err != nil || !strings.HasPrefix(key, "chat-media/") {
		t.Fatalf("chat canonical key = %q, %v", key, err)
	}
	if canonicalMIMEType != "video/mp4" {
		t.Fatalf("canonical MIME type = %q", canonicalMIMEType)
	}
}
