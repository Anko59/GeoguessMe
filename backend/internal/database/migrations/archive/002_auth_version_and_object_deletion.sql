-- 002_auth_version_and_object_deletion.sql
-- Adds per-user auth-versioning for immediate access-token revocation and a
-- durable queue so account/group deletion can never orphan stored media.
-- Forward-only and idempotent under advisory-locked migration execution.

ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS users_auth_version_idx ON users(auth_version);

CREATE TABLE IF NOT EXISTS media_deletion_jobs (
    id TEXT PRIMARY KEY,
    storage_key TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('account', 'group', 'retention', 'manual')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS media_deletion_jobs_pending_idx
    ON media_deletion_jobs(next_attempt_at)
    WHERE completed_at IS NULL;
