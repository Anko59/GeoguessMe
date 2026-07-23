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
| make bootstrap-integration            | Prepare the focused backend integration toolchain                                |
| make bootstrap-e2e                    | Prepare the focused browser toolchain                                            |
| make dev                              | Start the development stack                                                      |
| make format / make format-check       | Format or check tracked files                                                    |
| make preflight                        | Fast local/PR lint, contract, audit, type, and unit gate                         |
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
| make artifacts-clean                  | Remove workspace build/coverage/report artifacts (Dockerized, cache-safe)        |
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
| make hosted-config                    | Validate isolated dev/production hosted topology                                 |
| make terraform-validate / plan        | Validate or plan Hetzner/Cloudflare infrastructure                               |
| make hosted-contract-test             | Check deployment, backup, locking, proxy, and rollback contracts                 |

Pull requests target `dev`. Production release PRs target `main` from a
short-lived repository `release/*` branch whose Git tree must exactly equal the
successfully deployed `dev` tree. This avoids squash-history conflicts without
rewriting either protected branch. The protected branches accept verified squash
commits. PR CI classifies changed paths, runs the fast gate, and selects backend
integration or Chromium E2E. After merge, the exact `dev` revision runs the
complete gate once before deploy. Production promotes those signed image digests
without rebuilding them.

Run make help for the full target list. Use make build-images for normal
development; reserve make clean-build for reproducible CI or cache-invalidation
scenarios. Hooks use make pre-commit and the fast make pre-push gate; they fail
when Docker is unavailable and cannot be bypassed.

## Production platform

The repository includes a one-host Hetzner deployment for up to roughly 100
initial users. Dev and production are separate Compose projects with separate
PostgreSQL volumes, encrypted secrets, R2 buckets, ports, limits, and release
metadata. Cloudflare Tunnel provides outbound-only ingress; dev uses Access
email OTP, while production is public.

Local verification still cannot prove live R2, Cloudflare Access, or Brevo
delivery. Follow the hosted deployment runbook and complete dev acceptance plus
a real backup restore before declaring production ready. The shared host is a
single failure domain and deployments may interrupt service for up to two
minutes; no autoscaling or zero-downtime claim is made.

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

Production uses immutable signed backend and web image digests, a one-shot
migration service, non-root read-only backend execution, local persistent
PostgreSQL, private Cloudflare R2 media, and Brevo SMTP. Start with the
[hosted runbook](docs/runbooks/hosted-deployment.md), deployment guide, and
operations guide.

## Documentation

Start with the documentation index, testing guide, deployment guide, and
contributing guide. The machine-readable API contract is docs/openapi.yaml.
