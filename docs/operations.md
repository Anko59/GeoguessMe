# Operations

## Health endpoints

| Endpoint        | Expected status | Checks                                                                               |
| --------------- | --------------- | ------------------------------------------------------------------------------------ |
| `/health/live`  | 200 OK          | Always returns `ok\n`. No dependencies checked.                                      |
| `/health/ready` | 200 OK / 503    | PostgreSQL ping + object store `Health()` check. Returns `ready\n` or `not ready\n`. |
| `/health`       | Not available   | No catch-all health endpoint.                                                        |

Health check commands:

```bash
# Through the proxy
curl -s -o /dev/null -w '%{http_code}' http://localhost/health/live
curl -s -o /dev/null -w '%{http_code}' http://localhost/health/ready

# Direct to backend
curl -s -o /dev/null -w '%{http_code}' http://backend:8080/health/ready
```

## Metrics

OpenMetrics (Prometheus) format at `/metrics`:

| Metric                               | Type    | Description                  |
| ------------------------------------ | ------- | ---------------------------- |
| `geoguessme_http_requests_total`     | Counter | Total HTTP requests          |
| `geoguessme_http_errors_total`       | Counter | HTTP 5xx responses           |
| `geoguessme_storage_cleanup_backlog` | Gauge   | Pending object-deletion jobs |

The `/metrics` endpoint is unprotected in `development` and `test`. In
`production` (and any environment other than `development` or `test`) it
requires Bearer authentication with the `METRICS_TOKEN` value. The token must be
at least 32 bytes (`openssl rand -hex 32`) and is compared in constant time. A
failed authentication returns `401` with `WWW-Authenticate: Bearer` and
`Cache-Control: no-store` so load balancers and caches do not serve or store the
protected response.

```bash
# Production metrics require the configured bearer token
curl -s -H "Authorization: Bearer $METRICS_TOKEN" http://backend:8080/metrics
```

## Logging

The backend uses `slog` with JSON handler output. Every HTTP request is logged
with:

- `request_id` — from `X-Request-ID` header or auto-generated
- `method`, `path` — request line
- `status` — HTTP status code
- `duration_ms` — request duration in milliseconds

Log levels: `debug`, `info`, `warn`, `error` (default: `info`).

The server never logs tokens, signed URLs, passwords, exact coordinates, or
email links in text form.

## Backups

### Database

Use the Dockerized `make db-backup` target:

```bash
DATABASE_URL=postgres://... make db-backup
```

Or with a custom backup directory:

```bash
DATABASE_URL=postgres://... BACKUP_DIR=/mnt/backups make db-backup
```

The backup is a gzipped PostgreSQL dump with clean/if-exists options. Output
file: `{BACKUP_DIR}/geoguessme-{timestamp}.sql.gz`.

The script refuses to overwrite an existing backup file. Set `DATABASE_URL` or
the standard `PGHOST`/`PGPORT`/`PGUSER`/`PGDATABASE`/`PGPASSWORD` environment
variables.

### S3 media

S3 media must be backed up using the provider's versioning or replication
facilities. The database stores only object keys (not signed URLs), so database
dumps and S3 backups should be retained together to maintain referential
integrity.

## Restore verification

Restore into a separate database through the Dockerized `make db-restore`
target, verify with `make migrate-status`, and run a two-user smoke test before
replacing production.

The restore script requires interactive confirmation (type the target database
name). It is destructive — existing objects are dropped by `pg_restore`.

## Restart rehearsal

The `make restart-rehearsal` target runs a stateful restart rehearsal that
verifies all services recover cleanly with persistent data:

1. Starts the full test stack with a dedicated project name
2. Seeds real fixture data (users, groups, photos, guesses, messages, challenge
   views) and a MinIO media object
3. Records pre-restart state: row counts, migration count, data checksums,
   constraint count, and MinIO object content
4. Restarts **all** services (`docker compose down` without `-v`, then
   `docker compose up -d --wait`) — containers and networks are recreated, but
   named volumes persist
5. Polls health/readiness with deadline-based polling (no unconditional sleeps)
6. Verifies: schema continuity (migrations unchanged, no duplicates), data
   continuity (row counts and checksums match), media continuity (MinIO object
   intact), no runaway deletion jobs, and nominal metrics backlog
7. Cleans up all project resources on exit (success or failure)

The rehearsal is self-contained and uses the `geoguessme-restart-rehearsal`
project. It is safe to run locally; it refuses project names that do not contain
`rehearsal`. Regression tests validate the script structure via
`make test-restart-regression`.

## Docker resource pruning

Project-scoped Docker artifact and cache pruning is available through
`tools/quality/prune.sh` and the `make prune-report` / `make prune` targets.

### Safety guarantees

- **Dry-run by default**: `prune-report` (and `prune.sh --dry-run`) only report
  what would be removed.
- **Explicit confirmation**: `make prune` requires `CONFIRM=prune` to execute.
- **Project scope**: only resources matching `PROJECT_PREFIX=geoguessme` are
  targeted. The script refuses to run with a missing or ambiguous prefix.
- **No host-wide operations**: images and volumes are filtered by prefix;
  `docker builder prune` only removes dangling (unreferenced) build cache.
- **No arbitrary paths**: only known workspace artifact paths within the
  repository are removed.
- **Bounded**: refuses if the number of affected images exceeds `--max-images`
  (default: 50) or volumes exceed `--max-volumes` (default: 20).

### Usage

```bash
# Preview what would be pruned (safe, read-only)
make prune-report

# Execute pruning (requires explicit confirmation)
CONFIRM=prune make prune

# Include volumes (opt-in, data is destructive)
CONFIRM=prune make prune ARGS="--include-volumes"
```

### What is pruned

| Resource            | Scope                  | Opt-in                  | Command                                |
| ------------------- | ---------------------- | ----------------------- | -------------------------------------- |
| Project images      | `geoguessme*` prefix   | No (always)             | `docker rmi` per image                 |
| Dangling cache      | Host (dangling layers) | `--include-build-cache` | `docker builder prune --force`         |
| Project volumes     | `geoguessme*` prefix   | `--include-volumes`     | `docker volume rm` per volume          |
| Workspace artifacts | Known repo paths       | No (always)             | `rm -rf` on build/coverage/report dirs |

Volumes and build cache are opt-in because they cross into data-destructive
territory. Always run `make prune-report` first.

### Related targets

| Target                       | Description                                            |
| ---------------------------- | ------------------------------------------------------ |
| `make cache-status`          | Read-only report of project Docker resources           |
| `make prune-report`          | Dry-run preview of project-scoped pruning              |
| `make prune`                 | Execute project-scoped pruning (needs `CONFIRM=prune`) |
| `make clean`                 | Remove build artifacts and dangling build cache        |
| `make tools-clean`           | Remove tool containers, networks, and caches           |
| `make test-prune-regression` | Run pruning regression tests                           |

## Project disk cleanup

Guarded project filesystem cleanup is available through
`tools/quality/disk-cleanup.sh` and the `make disk-cleanup-report` /
`make disk-cleanup` targets. This is separate from Docker cache pruning and only
touches workspace artifact files, not Docker resources.

### Disk cleanup safety guarantees

- **Dry-run by default**: `disk-cleanup-report` (and
  `disk-cleanup.sh --dry-run`) only report what would be removed.
- **Explicit confirmation**: `make disk-cleanup` requires `CONFIRM=disk-cleanup`
  to execute.
- **Git-tracked preservation**: files tracked by Git are never removed.
- **Path allowlist**: only known artifact paths (build outputs, coverage
  reports, test results, E2E reports) are eligible for cleanup.
- **Dangerous path refusal**: the script refuses to operate on `/`, `/home`,
  `/root`, `/etc`, and other system directories. Ambiguous paths containing `..`
  are rejected.
- **Age bounds**: only files older than `--min-age-days` (default: 7) are
  eligible. Refuses non-numeric, zero, or negative values.
- **Size bounds**: refuses if the total eligible size exceeds `--max-total-mb`
  (default: 1024). This prevents accidental bulk deletion.
- **Unrelated project safety**: only paths under the validated allowlist are
  touched. Other project directories, source files, configuration, and
  documentation are never affected.

### Disk cleanup usage

```bash
# Preview what would be cleaned (safe, read-only)
make disk-cleanup-report

# Preview with custom age threshold
make disk-cleanup-report ARGS="--min-age-days 14"

# Execute cleanup (requires explicit confirmation)
CONFIRM=disk-cleanup make disk-cleanup

# Execute with custom bounds
CONFIRM=disk-cleanup make disk-cleanup ARGS="--min-age-days 30 --max-total-mb 500"
```

### What is cleaned

| Path                          | Type        | Git-tracked | Age filter |
| ----------------------------- | ----------- | ----------- | ---------- |
| `backend/bin/`                | Directory   | Preserved   | Yes        |
| `backend/tmp/`                | Recursive   | Preserved   | Yes        |
| `backend/coverage.out`        | Single file | Preserved   | Yes        |
| `frontend/dist/`              | Directory   | Preserved   | Yes        |
| `frontend/coverage/`          | Recursive   | Preserved   | Yes        |
| `frontend/test-results/`      | Recursive   | Preserved   | Yes        |
| `frontend/playwright-report/` | Recursive   | Preserved   | Yes        |
| `frontend/blob-report/`       | Recursive   | Preserved   | Yes        |

Files tracked by Git within these paths are always skipped. For recursive paths
(tmp, coverage, test-results, playwright-report, blob-report), each individual
file is checked against the age threshold and git-tracked status. Non-recursive
paths (bin, dist) are reported as a single unit.

### Disk cleanup related targets

| Target                              | Description                                           |
| ----------------------------------- | ----------------------------------------------------- |
| `make disk-cleanup-report`          | Dry-run preview of project disk cleanup               |
| `make disk-cleanup`                 | Execute disk cleanup (needs `CONFIRM=disk-cleanup`)   |
| `make test-disk-cleanup-regression` | Run disk-cleanup regression tests                     |
| `make clean`                        | Remove build artifacts and dangling build cache       |
| `make prune-report`                 | Dry-run preview of Docker resource pruning (separate) |

## Storage cleanup worker

A background goroutine runs immediately on startup and then every hour:

1. **Auth token cleanup**: deletes expired refresh sessions, verification
   tokens, password-reset tokens, and WebSocket tickets.
2. **Challenge-view expiry**: removes expired `challenge_views` rows older than
   one day.
3. **Retention media sweep**: finds photos past `retention_at`, marks them
   `removed`, and enqueues deletion jobs.
4. **Deletion queue**: claims up to 25 pending jobs with a 15-minute back-off,
   deletes the object from S3, and marks jobs complete.

Failures are logged at `WARN` level. The backlog is exposed via the
`geoguessme_storage_cleanup_backlog` metric.

## Secret rotation

### JWT secret

1. Update `JWT_SECRET` in `deployment/env/production.env`
2. Restart every API replica
3. Existing access tokens signed with the old secret are immediately invalid
4. Revoke all refresh sessions manually via SQL if needed

### SMTP credentials

1. Update `SMTP_USERNAME`/`SMTP_PASSWORD`
2. Restart API replicas
3. Inspect delivery logs for any unauthorised reset/verification requests
4. If links may have leaked, revoke all user sessions

### S3 credentials

1. Rotate keys in the object storage provider
2. Update `S3_ACCESS_KEY`/`S3_SECRET_KEY`
3. Restart API replicas
4. Inspect object access logs
5. If objects were exposed, invalidate them; media URLs are short-lived

## Incident response

| Scenario          | Response                                                                                                   |
| ----------------- | ---------------------------------------------------------------------------------------------------------- |
| Leaked JWT secret | Rotate secret, restart replicas, revoke refresh sessions                                                   |
| SMTP compromise   | Rotate credentials, inspect reset/verification delivery, revoke all user sessions if links may have leaked |
| S3 compromise     | Rotate keys, inspect access logs, invalidate exposed objects                                               |
| Abusive user      | Revoke sessions, delete account via `DELETE /auth/account`, preserve request IDs                           |
| Failed migration  | Restore from backup — migrations are forward-only                                                          |
| Storage outage    | Backend returns 502/503 on media endpoints; gameplay continues without media                               |
