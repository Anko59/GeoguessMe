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

```
001_initial.sql
002_auth_version_and_object_deletion.sql
```

New migrations:

```bash
make migration-new NAME=add_thing
# Creates backend/internal/database/migrations/003_add_thing.sql
```

## Migration commands

```bash
# Apply pending migrations
make migrate-up

# Show applied and pending migrations
make migrate-status

# Equivalent direct invocation
cd backend && go run . migrate up
cd backend && go run . migrate status
```

The server does NOT run migrations on startup. Deployments must run the
migration job explicitly before starting API processes.

## Migration 001: Initial schema

Creates all core tables with `IF NOT EXISTS` guards:

- `users` — with email/group field backfill from legacy schema
- `groups`, `group_members`
- `photos` — with storage_key/mime_type/byte_size/retention_at backfill
- `guesses` — with group_id backfill, dedupes before creating UNIQUE(photo_id, user_id)
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
