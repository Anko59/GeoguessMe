-- 003_unique_active_media_deletion_job.sql
-- Enforces at most one active (not-yet-completed) media-deletion job per storage
-- key so concurrent enqueues can never duplicate an in-flight deletion obligation.
-- The index is partial on completed_at IS NULL, so a completed job never blocks a
-- later job for the same key.
--
-- Forward-only and idempotent (per the migration rules): any duplicate active
-- jobs accumulated before this guard are collapsed first -- one survivor per key
-- (the stored object is still deleted exactly once) -- and the index is created
-- with IF NOT EXISTS so a partially-applied migration re-runs cleanly. Each
-- migration is applied inside a single transaction, so a failure rolls the whole
-- migration back. In-effect rollback is:
--   DROP INDEX IF EXISTS media_deletion_jobs_active_storage_key_idx;

DELETE FROM media_deletion_jobs
WHERE id IN (
    SELECT dupes.id
    FROM (
        SELECT
            id,
            ROW_NUMBER() OVER (
                PARTITION BY storage_key
                ORDER BY created_at, next_attempt_at, id
            ) AS rn
        FROM media_deletion_jobs
        WHERE completed_at IS NULL
    ) AS dupes
    WHERE dupes.rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS media_deletion_jobs_active_storage_key_idx
    ON media_deletion_jobs (storage_key)
    WHERE completed_at IS NULL;
