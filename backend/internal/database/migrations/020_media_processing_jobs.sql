-- Media-processing jobs for quarantined asynchronous video processing.
--
-- Video uploads are stored under private quarantine keys and processed by a
-- dedicated worker instead of being served immediately. The job row carries
-- the opaque upload context (metadata) needed to create the challenge or chat
-- record atomically once the canonical object has been written. Jobs are
-- retained for 24 hours (expires_at) so the owner can poll the status
-- endpoint, then purged by the cleanup runner.
CREATE TABLE IF NOT EXISTS media_processing_jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('challenge', 'chat')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'ready', 'failed')),
    quarantine_key TEXT NOT NULL,
    canonical_key TEXT,
    result_kind TEXT,
    result_id TEXT,
    mime_type TEXT,
    byte_size INTEGER,
    error_code TEXT,
    group_id TEXT,
    metadata JSONB,
    worker_id TEXT,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours'),
    CONSTRAINT media_processing_jobs_consistency CHECK (
        (status = 'failed' AND error_code IS NOT NULL)
        OR (status <> 'failed' AND error_code IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS media_processing_jobs_user_idx ON media_processing_jobs(user_id);
CREATE INDEX IF NOT EXISTS media_processing_jobs_claim_idx ON media_processing_jobs(status, queued_at) WHERE status IN ('queued', 'processing');
CREATE INDEX IF NOT EXISTS media_processing_jobs_expiry_idx ON media_processing_jobs(expires_at) WHERE status IN ('ready', 'failed');
