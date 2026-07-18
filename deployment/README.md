# GeoGuessMe deployment

This repository has separate Docker Compose topologies for development, tests,
and production, plus a Docker-only tool topology in
deployment/compose.tools.yaml.

## Tool-container architecture

go-tools supplies Go, goimports, and GolangCI-Lint for formatting, linting,
testing, and building. go-security is a separate heavier image that isolates
specialized security and operations tools: govulncheck, the CGO build chain (for
race detection), and PostgreSQL client utilities (psql, pg_dump). node-tools
supplies the locked frontend dependencies, Prettier, ESLint, Stylelint,
Markdownlint, Redocly, TypeScript, Vitest, and Axe. The official Playwright
image matches the locked @playwright/test version. ShellCheck, shfmt, Hadolint,
actionlint, SQLFluff, and Caddy each run in their own pinned image.

Tool services mount the repository at /workspace, use named caches, do not
receive production secrets, application volumes, or the Docker socket, and use
read-only source mounts for checks. Formatter services use the host UID/GID.

## First deploy

Set immutable image references (`BACKEND_IMAGE` and `WEB_IMAGE` must include an
`@sha256:...` digest) and create the ignored production environment file from
deployment/env/production.env.example.

```text
make compose-validate
make prod-config
make prod-migrate
make prod-up
make smoke BASE_URL=https://your-domain.example
```

The migration job must complete before the backend starts. The backend runs
non-root with a read-only root filesystem, /tmp tmpfs, a health check, and
graceful termination. Caddy proxies the API and WebSocket endpoint same-origin.

## Migrations and upgrades

Use `make migrate-status`, `make migrate-up`, and
`make migration-new NAME=description`. Migration acquisition uses a PostgreSQL
advisory lock; `make migration-test` runs concurrent migration processes to
prove the lock works, then re-runs to prove idempotency against a legacy
fixture.

For PostgreSQL version upgrades, run `make db-backup`, provision an empty target
database, restore through `make db-restore FILE=...`, run `make migrate-status`,
and exercise `make smoke` against the restored application. Keep database and S3
backups together because the database stores media object keys.

## Rollback and restart

Rollback deploys a previously known-good immutable image. Migrations are
forward-only; restore the pre-upgrade backup when a migration cannot be
supported by the previous binary. `make prod-down` followed by `make prod-up` is
a restart, not a zero-downtime rolling deployment. Rolling deployment requires
an orchestrator and is not claimed by this Compose setup.

## Rehearsal evidence

The following targets provide deterministic, disposable evidence:

| Target                     | Evidence                                                                                     |
| -------------------------- | -------------------------------------------------------------------------------------------- |
| make backup-rehearsal      | Seeds data, dumps, restores into fresh DB, verifies rows, constraints, migrations, checksums |
| make restart-rehearsal     | Proves schema, data, media, and metric continuity across full container restart              |
| make reconnect-rehearsal   | Proves WebSocket catch-up, live delivery, exact-once messages after disconnect               |
| make migration-test        | Proves advisory-lock concurrency, idempotency, legacy backfill, deduplication                |
| make prod-container-verify | Proves non-root, healthcheck, read-only, compose, HTTP smoke                                 |
| make load-test             | Proves k6 thresholds: <1% failures, p(95) <500ms, 100% websocket delivery                    |
| make smoke-rehearsal       | Proves HTTP liveness, readiness, auth enforcement, game flow against disposable stack        |

All rehearsals are fully Dockerized, use disposable project names, and clean up
on exit (success or failure).

## Backups and outage response

For an outage, inspect `make prod-logs`, `/health/live`, `/health/ready`, and
`/metrics`. Preserve request IDs and logs, check PostgreSQL and object storage
health, and do not delete data or rotate secrets before preserving evidence.

Use `make smoke`, `make backup-rehearsal`, and `make load-test` only against
disposable local/test or explicitly selected staging environments — never
against production.

## Known unproven production inputs

No rehearsal or container-verify exercise touches external PostgreSQL, S3, or
SMTP infrastructure. All live-stack tests use Compose-local services (local-db,
local-minio, local-smtp profiles). Deployers must validate:

- External PostgreSQL connection, TLS, and performance
- External S3 endpoint, bucket creation, credentials
- Authenticated SMTP delivery and TLS
- TLS termination at the edge with proper certificate management
- Secrets management (vault, SOPS, or env file as chosen)

See docs/configuration.md, docs/testing.md, and docs/operations.md.
