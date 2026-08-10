package models

import "time"

// MediaProcessingKind is the target kind of an asynchronous media upload:
// challenge photos or chat attachments.
type MediaProcessingKind string

const (
	MediaProcessingKindChallenge MediaProcessingKind = "challenge"
	MediaProcessingKindChat      MediaProcessingKind = "chat"
)

// MediaProcessingStatus is the lifecycle state of a processing job.
type MediaProcessingStatus string

const (
	MediaProcessingStatusQueued     MediaProcessingStatus = "queued"
	MediaProcessingStatusProcessing MediaProcessingStatus = "processing"
	MediaProcessingStatusReady      MediaProcessingStatus = "ready"
	MediaProcessingStatusFailed     MediaProcessingStatus = "failed"
)

// MediaProcessingJob is the persisted row for a quarantined asynchronous video
// upload. Storage keys and upload metadata are internal state, never exposed
// through the owner-facing response (see ToResponse).
type MediaProcessingJob struct {
	ID            string
	UserID        string
	Kind          MediaProcessingKind
	Status        MediaProcessingStatus
	QuarantineKey string
	CanonicalKey  string
	ResultKind    string
	ResultID      string
	MIMEType      string
	ByteSize      int64
	ErrorCode     string
	GroupID       string
	Metadata      []byte // opaque upload context (JSON), internal only
	WorkerID      string
	QueuedAt      time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	ExpiresAt     time.Time
}

// MediaProcessingResult is the compact resolved media reference a ready job
// carries when its full record cannot be resolved (for example after the
// media was purged). The status endpoint enriches it with the actual
// challenge or chat message result when the record still exists.
type MediaProcessingResult struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	MIMEType string `json:"mime_type,omitempty"`
	ByteSize int64  `json:"byte_size,omitempty"`
}

// ChallengeProcessingMetadata is the opaque upload context the worker needs
// to reconstruct the per-group challenge records once the canonical object
// exists. It is serialized into MediaProcessingJob.Metadata and never
// surfaced through the owner-facing response.
type ChallengeProcessingMetadata struct {
	GroupIDs     []string `json:"group_ids"`
	Lat          float64  `json:"lat"`
	Long         float64  `json:"long"`
	HideLocation bool     `json:"hide_location"`
}

// ChatProcessingMetadata is the opaque upload context the worker needs to
// reconstruct the chat message once the canonical object exists. It is
// serialized into MediaProcessingJob.Metadata and never surfaced through the
// owner-facing response.
type ChatProcessingMetadata struct {
	GroupID   string  `json:"group_id"`
	Content   string  `json:"content"`
	ReplyToID *string `json:"reply_to_id,omitempty"`
}

// MediaProcessingJobResponse is the owner-facing status payload returned by
// GET /api/v1/media-processing/{jobID}. Quarantine/canonical storage keys and
// the opaque upload metadata are deliberately never serialized; the result is
// present only when the job is ready and the error code only when it failed.
type MediaProcessingJobResponse struct {
	ID          string                `json:"id"`
	Kind        MediaProcessingKind   `json:"kind"`
	Status      MediaProcessingStatus `json:"status"`
	QueuedAt    time.Time             `json:"queued_at"`
	StartedAt   *time.Time            `json:"started_at,omitempty"`
	CompletedAt *time.Time            `json:"completed_at,omitempty"`
	Result      any                   `json:"result,omitempty"`
	ErrorCode   string                `json:"error_code,omitempty"`
}

// ToResponse maps the persisted job to the safe owner-facing payload.
func (j *MediaProcessingJob) ToResponse() MediaProcessingJobResponse {
	resp := MediaProcessingJobResponse{
		ID:          j.ID,
		Kind:        j.Kind,
		Status:      j.Status,
		QueuedAt:    j.QueuedAt,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
	}
	switch j.Status {
	case MediaProcessingStatusReady:
		resp.Result = &MediaProcessingResult{
			Kind:     j.ResultKind,
			ID:       j.ResultID,
			MIMEType: j.MIMEType,
			ByteSize: j.ByteSize,
		}
	case MediaProcessingStatusFailed:
		resp.ErrorCode = j.ErrorCode
	}
	return resp
}
