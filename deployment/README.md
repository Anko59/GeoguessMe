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

## Hosted topology

The hosted overlay runs `dev` and `production` on one Hetzner CX23 as separate
Compose projects. Each has an isolated PostgreSQL volume, environment file, R2
media bucket, loopback port, and resource limits. Cloudflare Tunnel is the only
ingress path: production uses port 8081, dev uses 8082, and public inbound
firewall rules are empty. Dev is protected by Cloudflare Access email OTP for
the owner; CI has a separate service-token rule used only for health checks and
deployment SSH.

Terraform in `infra/terraform` provisions the server, backups, deletion
protection, deny-inbound firewall, tunnel routes, DNS, Access applications, and
private R2 buckets. State belongs in the private R2 state bucket with an S3
lockfile. Bucket-scoped R2 S3 credentials are deliberately created outside
Terraform so their secret values never enter state.

## Bootstrap and first deploy

Complete the ordered checklist in
[`docs/runbooks/hosted-deployment.md`](../docs/runbooks/hosted-deployment.md).
The main operator targets are:

```text
make terraform-validate
make terraform-plan
CONFIRM=apply make terraform-apply
make secrets-generate ENV=dev RECIPIENT=age1...
make hosted-config
```

Cloud-init creates independent age identities for dev and production. SOPS
encrypted dotenv files are tracked as `deployment/secrets/*.env.enc`; decrypted
files exist only as mode-0600 files under `/etc/geoguessme` on the host.
`secrets-generate` requires the documented SMTP, GHCR, media-R2, backup-R2, and
Cloudflare account variables and streams plaintext directly between the pinned
tool and SOPS containers, so no plaintext dotenv file is created locally.

The host deployment command accepts only an environment fixed in
`authorized_keys`, two digest-qualified image references, and a 40-character
commit. It verifies GitHub Actions keyless signatures before pull, serializes
both stacks with one lock, creates a pre-deploy backup, migrates, waits for
health, and records the active release. Application failure restores previous
images only; it never automatically restores PostgreSQL.

Development merges run the complete operational gate exactly once, then build,
attest, and sign immutable images before deployment. A release PR may come only
from a repository `release/*` branch whose tree exactly equals the successfully
deployed `dev` tree. Basing that short-lived branch on `main` avoids recurring
squash-history conflicts without rewriting either protected branch. Production
compares the `main` and `dev` Git trees, verifies the development workflow
signatures, promotes the exact manifests without rebuilding, verifies unchanged
digests, adds the production workflow signature, and deploys those references.

## Generic first deploy

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

## Hosted backups and outage response

Hosted PostgreSQL is dumped hourly, compressed, and encrypted by Restic into a
private R2 bucket. Retention is 24 hourly, 14 daily, 8 weekly, and 6 monthly
snapshots. A production restore is rehearsed weekly in a disposable PostgreSQL
container. `geoguessme-health@*.timer` checks application/container health, disk
pressure, tunnel state, and a maximum backup age of two hours every 15 minutes.

For an outage, inspect `make prod-logs`, `/health/live`, `/health/ready`, and
`/metrics`. Preserve request IDs and logs, check PostgreSQL and object storage
health, and do not delete data or rotate secrets before preserving evidence.

Use `make smoke`, `make backup-rehearsal`, and `make load-test` only against
disposable local/test or explicitly selected staging environments — never
against production.

## Live acceptance boundary

Local verification does not prove Cloudflare R2, Access OTP, or Brevo delivery.
Before production release, validate these on dev and complete a real encrypted
production-backup restore rehearsal. Never run the disposable/destructive smoke
suite against production.

- R2 endpoint, private bucket access, uploads, and media reads
- Authenticated SMTP delivery and TLS
- Cloudflare TLS, Access OTP, WebSockets, and trusted client-IP propagation
- Backup freshness, restore evidence, and release rollback metadata

See docs/configuration.md, docs/testing.md, and docs/operations.md.
