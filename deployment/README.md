# GeoGuessMe deployment

This repository has separate Docker Compose topologies for development, tests,
and production, plus a Docker-only tool topology in
deployment/compose.tools.yaml.

## Tool-container architecture

go-tools supplies Go, goimports, GolangCI-Lint, govulncheck, PostgreSQL client
utilities, and test/build tools. node-tools supplies the locked frontend
dependencies, Prettier, ESLint, Stylelint, Markdownlint, Redocly, TypeScript,
Vitest, and Axe. The official Playwright image matches the locked
@playwright/test version. ShellCheck, shfmt, Hadolint, actionlint, SQLFluff, and
Caddy each run in their own pinned image.

Tool services mount the repository at /workspace, use named caches, do not
receive production secrets, application volumes, or the Docker socket, and use
read-only source mounts for checks. Formatter services use the host UID/GID.

## First deploy

Set immutable image references and create the ignored production environment
file from deployment/env/production.env.example.

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

Use make migrate-status, make migrate-up, and make migration-new
NAME=description. Migration acquisition uses a database advisory lock;
concurrent migration rehearsals must show each migration applied once and a
second run idempotent.

For PostgreSQL upgrades, run make db-backup, provision an empty target, restore
through make db-restore FILE=..., run make migrate-status, and exercise make
smoke against the restored application. Keep database and S3 backups together
because the database stores media object keys.

## Rollback and restart

Rollback deploys a previously known-good immutable image. Migrations are
forward-only; restore the pre-upgrade backup when a migration cannot be
supported by the previous binary. make prod-down followed by make prod-up is a
restart rehearsal, not a zero-downtime rolling deployment. Rolling deployment
requires an orchestrator and is not claimed by this Compose setup.

## Backups and outage response

make backup-rehearsal seeds representative disposable data, creates a
containerized backup, restores into a fresh database, and verifies rows,
constraints, migrations, checksums, and application reads. make
restart-rehearsal verifies refresh sessions, WebSocket catch-up, and stored data
across a restart.

For an outage, inspect make prod-logs, /health/live, /health/ready, and
/metrics. Preserve request IDs and logs, check PostgreSQL and object storage
health, and do not delete data or rotate secrets before preserving evidence. Use
make smoke, make backup-rehearsal, and the documented load profile only against
disposable local/test or explicitly selected staging environments.

See docs/configuration.md, docs/testing.md, and docs/operations.md.
