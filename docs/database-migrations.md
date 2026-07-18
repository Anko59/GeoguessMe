# Database migrations

## Migration rules

1. **Forward-only**: Once applied, a migration is never modified or reverted.
   Rollback is done by deploying a previous binary version and restoring from
   backup if needed.
2. **Embedded SQL**: Migrations are `.sql` files embedded in the Go binary via
   `//go:embed` in `backend/internal/database/db.go`.
3. **`schema_migrations` table**: Tracks which migrations have been applied
   (version, name, applied_at).
4. **Advisory lock**: Migrations acquire PostgreSQL advisory lock key `91734721`
   to prevent concurrent execution across replicas. The lock is held for the
   entire migration run.
5. **Idempotent**: Every SQL file uses `IF NOT EXISTS`, `IF EXISTS`, and
   backfill `UPDATE` statements so re-running a partially-failed migration (or
   applying it against an already-schema-compatible database) is safe.

## File naming

Migrations are stored in `backend/internal/database/migrations/`:

```text
001_initial.sql
002_auth_version_and_object_deletion.sql
003_unique_active_media_deletion_job.sql
```

New migrations:

```bash
make migration-new NAME=add_thing
# Creates backend/internal/database/migrations/004_add_thing.sql
```

## Migration commands

```bash
# Apply pending migrations
make migrate-up

# Show applied and pending migrations
make migrate-status

# Run deterministic concurrent, idempotent, and legacy-fixture migration tests
make migration-test

# There is no supported host-side direct invocation. Use the Make targets so
# the migration binary runs in the Docker tool/application container.
```

The server does NOT run migrations on startup. Deployments must run the
migration job explicitly before starting API processes.

## Migration test target

`make migration-test` starts from a representative legacy database state
(pre-migration 001), runs two concurrent migration processes to verify advisory
locking, then re-runs to prove idempotency. It verifies every backfill path and
column addition for every legacy row, then validates the migration 003
duplicate-survivor ORDER BY logic with a direct SQL replay.

The legacy fixture lives at `deployment/scripts/legacy-migration-fixture.sql`
and models the exact pre-migration schema: users without email/auth_version
columns (but with `score` which 001 drops), photos without
storage_key/retention_at, guesses without group_id, and messages without
kind/photo_id. Edge cases include NULL URLs, NULL expires_at, whitespace-heavy
usernames, and multi-row backfill paths.

The test is fully Dockerized (no host DB tools, no sleeps or retries). It uses
the same test Compose stack as the integration suite but with an elevated
`GEOGUESSME_MIGRATION_DB_PORT` to avoid port collisions.

## Migration 001: Initial schema

Creates all core tables with `IF NOT EXISTS` guards:

- `users` — with email/group field backfill from legacy schema
- `groups`, `group_members`
- `photos` — with storage_key/mime_type/byte_size/retention_at backfill
- `guesses` — with group_id backfill, dedupes before creating UNIQUE(photo_id,
  user_id)
- `messages` — with kind/photo_id backfill
- `challenge_views`
- `refresh_sessions`, `email_verification_tokens`, `password_reset_tokens`,
  `websocket_tickets`

Also adds foreign key constraints and indexes where missing.

## Migration 002: Auth version and object deletion

- Adds `auth_version INTEGER NOT NULL DEFAULT 0` to `users` for immediate
  access-token revocation on password reset or `logout?all=1`.
- Creates `media_deletion_jobs` table for durable async object storage cleanup,
  with a partial index on `(next_attempt_at) WHERE completed_at IS NULL`.

## Migration 003: Unique active media-deletion job

- Adds a partial unique index `media_deletion_jobs_active_storage_key_idx` on
  `media_deletion_jobs(storage_key)` `WHERE completed_at IS NULL`, so at most
  one active deletion job can exist per storage key. A completed job is outside
  the index, so a later job for the same key is still allowed.

### Lock/duplicate survivor behavior

The migration deduplication query uses
`ROW_NUMBER() OVER (PARTITION BY storage_key ORDER BY created_at, next_attempt_at, id)`
to select one survivor per key from any pre-existing active duplicates. The
survivor is the row with the earliest `created_at` (then `next_attempt_at`, then
`id` as tie-breaker). All other active rows for the same key are deleted.

At runtime the repository layer uses
`ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING` on every
insert:

- **Concurrent enqueue survivor**: When two callers (e.g. account deletion and
  retention sweep) attempt to insert an active job for the same key
  simultaneously, the partial unique index prevents the second insert. The
  first-inserted row survives; the second is a silent no-op. The stored object
  is still deleted exactly once because the single surviving job covers the
  obligation.
- **RetireRetainedMedia atomic lock**: The `RetireRetainedMedia` operation
  acquires `FOR UPDATE` on the photo row before deciding to insert a deletion
  job. If an active job already exists for that key, `ON CONFLICT DO NOTHING`
  prevents a duplicate. The row lock ensures no two sweeps can race past the
  lifecycle-status check.
- **Completed jobs are outside the index**: Once a job's `completed_at` is set,
  it falls outside the partial index, so a new active job for the same key can
  be inserted. This handles the case where the same object is re-uploaded and
  later deleted again.

Repository insertion uses
`ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING`, so a
duplicate active obligation is an idempotent success while genuine database
errors still fail.

## Status command

`make migrate-status` prints each migration with its version, name, and applied
timestamp. Any migration without a corresponding row in `schema_migrations`
appears without an applied timestamp — the binary will apply it on the next
`migrate up` run.

## Compatibility

The migration system is compatible with PostgreSQL 15 and 16.

## Recovery from failed migrations

1. The advisory lock prevents concurrent runs.
2. Each migration runs inside a single transaction. If the transaction fails,
   the migration is rolled back entirely.
3. To recover, fix the cause of failure (e.g. disk space, permissions) and
   re-run `make migrate-up`. The migration will retry since no row was inserted
   into `schema_migrations`.
