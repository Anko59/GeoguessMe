# Video processing

Video uploads are processed asynchronously. Uploading a video returns
`202 Accepted` with a media-processing job instead of creating a visible
challenge/chat message immediately; the raw source is stored under a private
`quarantine/*` object key and is never served. A worker validates and transcodes
the video into a canonical MP4, then atomically creates the challenge or chat
record and announces it exactly once.

This document describes the pipeline, its guarantees, and the runtime
requirements. The API contract lives in [openapi.yaml](openapi.yaml);
operational metrics and alerting live in [operations](operations.md).

## Upload contract

- **Image uploads** keep the existing synchronous behavior (normalized image,
  immediate challenge/chat creation, `201`).
- **Video uploads** are detected by MIME type and content bytes. The handler
  stores the raw source under a quarantine object key, inserts a
  `media_processing_job` row with status `queued`, and returns `202` with the
  safe job DTO (id, kind, status, timestamps). No challenge/chat record is
  created and nothing is broadcast or pushed at upload time. A job-insert
  failure compensates the quarantine object (deletes it) so no orphaned raw
  media survives.
- The owner polls `GET /api/v1/media-processing/{jobID}` with bounded 1–5 s
  backoff while the status is `queued` or `processing`, stops on `ready` or
  `failed`, and also stops on logout, navigation, or unmount. The frontend
  strips the invite-style fragment and never re-polls after completion.

## The worker

A single in-process worker goroutine in the backend server claims jobs from the
database with `SELECT ... FOR UPDATE SKIP LOCKED`, so a restarting worker never
collides with itself. The product runs one backend replica (see F-04), so the
worker shares the realtime hub and push notifier and a completed record is
announced exactly once.

### Validation (ffprobe)

Each quarantine source is inspected with `ffprobe` and must satisfy:

- exactly one video stream;
- at most one audio stream;
- no subtitle, attachment, or data streams;
- duration ≤ 30 s (malformed or missing duration is rejected);
- dimensions ≤ 1280×720;
- frame rate ≤ 30 fps;
- input size ≤ the configured upload cap (`UPLOAD_MAX_BYTES`, 10 MiB);
- supported, decodable codecs.

Rejections map to a stable, non-sensitive `error_code` on the job (for example
`invalid_duration`, `invalid_dimensions`, `unsupported_codec`,
`unexpected_streams`). The reason is never leaked to other users.

### Transcode (ffmpeg)

Valid sources are transcoded to the canonical format:

- MP4 container, H.264 video with `yuv420p` pixel format;
- AAC-LC audio when an audio stream exists;
- metadata and chapters stripped;
- `+faststart` layout (moov atom moved to the front).

The canonical object is written to a private `canonical/*` object key. Only
after the canonical object is durably written does the worker, in one
transaction, create the challenge or chat record and mark the job `ready`.
Broadcast and Web Push fire once, after the transaction commits. The raw
quarantine source and any partial canonical object are then enqueued for durable
deletion through the media-deletion job queue.

On failure or timeout the job transitions to `failed` with a stable
`error_code`, and both the partial canonical object and the quarantine source
are cleaned up through the durable deletion queue.

### Resource bounds

Each `ffprobe`/`ffmpeg` child is bounded by the worker's rlimit trampoline to:

- 60 s of CPU time (plus a caller context deadline for wall-clock time);
- 512 MiB of address space;
- 128 child processes.

The backend container (see `deployment/compose.production.yaml`) is non-root,
has a read-only root filesystem with a `/tmp` tmpfs for worker temp files, and
is resource-limited above the per-job backstop (defaults: 2.0 CPU, 1 GiB memory,
256 PIDs; overridable with `GEOGUESSME_BACKEND_CPUS`,
`GEOGUESSME_BACKEND_MEMORY`, `GEOGUESSME_BACKEND_PIDS`). The runtime image must
ship `ffmpeg`/`ffprobe` and a shell (the worker re-execs its own binary as an
rlimit trampoline); see `deployment/docker/backend.Dockerfile`.

### Lifecycle and cleanup

- Jobs are retained for 24 hours after completion or failure so the owner can
  poll the result, then are purged.
- Quarantine objects whose jobs were abandoned (for example the worker was
  stopped mid-download) are purged after one hour.
- The cleanup runner requeues stale `processing` jobs whose worker died.

## Architecture note

The worker deliberately runs as an **in-process goroutine** inside the backend
server rather than a separate container. A completed challenge/chat record must
be broadcast over WebSocket exactly once, and the WebSocket hub is in-memory in
the server process; a separate worker process would need a broadcast-outbox that
the server drains, which is a future option if the product ever runs more than
one backend replica or a dedicated worker container. The quarantine, validation,
transcode, atomic promotion, and cleanup guarantees above do not depend on that
choice.

## Configuration

| Variable                  | Default             | Notes                                                                    |
| ------------------------- | ------------------- | ------------------------------------------------------------------------ |
| `MEDIA_PROCESSING_WORKER` | `true`              | Starts the worker goroutine. The runtime image must ship ffmpeg/ffprobe. |
| `UPLOAD_MAX_BYTES`        | `10485760` (10 MiB) | Caps each image and video input; also the worker's input cap.            |

The frontend enables its video-recording camera UI only when the platform
reports a camera; see [gameplay](gameplay.md) for capture behavior.
