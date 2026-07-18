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
