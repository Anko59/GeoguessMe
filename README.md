# GeoGuessMe

GeoGuessMe is a real-time multiplayer photo-location guessing game. Group
members share short-lived private photo challenges, view each photo during a
server-controlled window, and submit one guess.

## Supported workflow

The only host prerequisites are Git, Make, Docker, and Docker Compose. Go, Node,
npm, Playwright, Python, linters, formatters, and migration tools run only in
the pinned Docker tool containers.

```text
make bootstrap
make dev
```

make bootstrap builds or pulls the pinned tool images, populates named
dependency caches from the lockfiles, installs the tracked hooks, and runs a
tool self-test. make dev starts PostgreSQL, MinIO, Mailpit, the backend, and the
Vite development server.

## Canonical commands

| Command                               | Purpose                                                                          |
| ------------------------------------- | -------------------------------------------------------------------------------- |
| make bootstrap                        | Prepare the Docker-only toolchain and hooks                                      |
| make dev                              | Start the development stack                                                      |
| make format / make format-check       | Format or check tracked files                                                    |
| make quality                          | Run all quality gates (structure, lint, audit, unit, race, coverage, build)      |
| make verify                           | Run the complete release gate (quality + live-stack + rehearsals + load)         |
| make test-unit / make test-race       | Run unit or race tests                                                           |
| make test-integration / make test-e2e | Run isolated live-stack suites                                                   |
| make build-images                     | Build production images with normal Docker layer caching                         |
| make clean-build                      | Build production images from scratch without layer cache                         |
| make cache-status                     | Report project-only Docker resources (read-only)                                 |
| make prune-report / make prune        | Preview or execute project-scoped Docker prune (CONFIRM=prune)                   |
| make disk-cleanup-report              | Preview project disk artifact cleanup (dry-run, read-only)                       |
| make disk-cleanup                     | Clean project disk artifacts (requires CONFIRM=disk-cleanup)                     |
| make tools-clean                      | Remove tool caches and containers                                                |
| make build-cache-prune                | Remove dangling build cache to prevent unbounded growth                          |
| make compose-validate                 | Validate every Compose file                                                      |
| make container-verify                 | Verify image hardening and health checks                                         |
| make prod-container-verify            | Full production-container verification (build, harden, compose, smoke, teardown) |
| make migration-test                   | Concurrent, idempotent, and legacy-fixture migration tests                       |
| make backup-rehearsal                 | Disposable backup/restore rehearsal with continuity evidence                     |
| make restart-rehearsal                | Stateful restart rehearsal (schema, data, media continuity)                      |
| make reconnect-rehearsal              | WebSocket disconnect/reconnect and exact-once evidence                           |
| make load-test                        | Disposable load test (k6, 5 VUs, 30s by default)                                 |
| make smoke / make smoke-rehearsal     | Smoke test against a staging or disposable stack                                 |

Run make help for the full target list. Use make build-images for normal
development; reserve make clean-build for reproducible CI or cache-invalidation
scenarios. Hooks use make pre-commit and make pre-push; they fail when Docker is
unavailable and cannot be bypassed.

## Known constraints (unproven in production)

The rehearsal and container-verify evidence proves correctness against
local/disposable infrastructure only. The following production inputs have no
test evidence in this repository:

- **External PostgreSQL, S3, and SMTP** — every live-stack test uses
  Compose-local services (local-db, local-minio, local-smtp profiles).
- **TLS termination at edge** — no ingress-controller, load-balancer, or
  certificate-provisioning configuration is shipped.
- **Zero-downtime rolling deployment** — Compose restart stops all containers
  before starting new ones.
- **Autoscaling** — no horizontal pod autoscaler or replica-count automation.
- **Secrets management** — production secrets are read from a single git-ignored
  `.env` file; no vault, SOPS, or external secrets store.

Deployers must validate these in their own infrastructure before claiming
production readiness.

## Architecture

- React, TypeScript, and Vite frontend served by Caddy.
- Go HTTP/WebSocket API with embedded ordered SQL migrations.
- PostgreSQL for accounts, groups, messages, challenges, and sessions.
- Private S3-compatible media storage; MinIO is used by development and tests.
- Authenticated SMTP; Mailpit is used by development and tests.

The test stack computes one public URL from its selected web port and passes it
to the backend, browser runner, WebSocket origin checks, and email links. The
Mailpit host port is passed independently with MAILPIT_BASE_URL.

## Deployment

Production uses immutable backend and web image references, a one-shot migration
service, non-root read-only backend execution, health checks, and external
PostgreSQL, S3, and authenticated SMTP by default. Use the deployment guide and
operations guide. Compose restart is a restart, not zero-downtime rolling
deployment.

## Documentation

Start with the documentation index, testing guide, deployment guide, and
contributing guide. The machine-readable API contract is docs/openapi.yaml.
