# Testing

All project test runners execute in Docker. The host invokes Make targets only;
the tool stack supplies Go, Node, Vitest, Playwright, Axe, linters, and coverage
tools from pinned images and named caches.

## Gates and expected results

| Target                                     | Scope                                                                                                                                                                              | Expected result                                                           |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| make test-unit                             | Backend unit tests and frontend Vitest                                                                                                                                             | go test PASS; Vitest PASS                                                 |
| make test-race                             | Backend race detector                                                                                                                                                              | go test -race PASS (no races)                                             |
| make test-structure-regression             | Structure-check regression tests in disposable Git repos                                                                                                                           | check-structure-regression.sh PASS                                        |
| make test-ci-retention-regression          | Verify CI bounded retention and cache scopes                                                                                                                                       | check-ci-retention-regression.sh PASS                                     |
| make test-cache-status-regression          | Cache-status reporting regression tests                                                                                                                                            | check-cache-status-regression.sh PASS                                     |
| make test-restart-regression               | Restart-rehearsal script structure regression                                                                                                                                      | check-restart-regression.sh PASS                                          |
| make test-prune-regression                 | Prune script regression tests                                                                                                                                                      | check-prune-regression.sh PASS                                            |
| make test-migration-fixture-regression     | Migration fixture structural regression                                                                                                                                            | check-migration-fixture-regression.sh PASS                                |
| make test-prod-container-verify-regression | Prod-container-verify script structure regression                                                                                                                                  | check-prod-container-verify-regression.sh PASS                            |
| make test-disk-cleanup-regression          | Disk-cleanup regression tests                                                                                                                                                      | check-disk-cleanup-regression.sh PASS                                     |
| make test-build-caching                    | Verify build-images uses layer caching and clean-build does not                                                                                                                    | check-build-caching.sh PASS                                               |
| make cache-status                          | Report project-only Docker resources (read-only)                                                                                                                                   | script output (non-fatal)                                                 |
| make coverage                              | Backend ≥70% overall; frontend ≥80/80/80/70 (statements/branches/functions/lines)                                                                                                  | go test cover OK; Vitest --coverage PASS                                  |
| make test-integration                      | Isolated PostgreSQL, MinIO, Mailpit, backend suite                                                                                                                                 | All integration tests PASS                                                |
| make test-e2e                              | Desktop and Pixel 5 Playwright projects                                                                                                                                            | All Playwright projects PASS                                              |
| make quality                               | Structure, format, lint, type-check, audit, regression, unit, race, coverage, build, compose-validate                                                                              | Zero violations; all gates PASS                                           |
| make migration-test                        | Concurrent, idempotent, legacy-fixture migration tests (advisory lock, backfill, dedupe)                                                                                           | migration-concurrency.sh PASS                                             |
| make backup-rehearsal                      | Disposable backup, restore, continuity verification                                                                                                                                | backup-restore-rehearsal.sh PASS                                          |
| make restart-rehearsal                     | Stateful restart: schema/data/media continuity, no duplicate migrations or jobs                                                                                                    | restart-rehearsal.sh: all checks PASS                                     |
| make reconnect-rehearsal                   | WebSocket disconnect/reconnect, cursor catch-up, exact-once delivery                                                                                                               | reconnect-rehearsal harness PASS                                          |
| make load-test                             | k6 profile (5 VUs, 30s by default)                                                                                                                                                 | k6 thresholds: http_req_failed <1%, p(95) <500ms, websocket delivery 100% |
| make container-verify                      | Runtime image hardening (non-root user, healthcheck, Compose validation)                                                                                                           | container-verify.sh PASS                                                  |
| make prod-container-verify                 | Production-image hardening, compose, local stack startup, smoke, teardown                                                                                                          | prod-container-verify.sh PASS                                             |
| make smoke-rehearsal                       | Smoke test against a disposable test stack                                                                                                                                         | smoke-rehearsal.sh PASS                                                   |
| make verify                                | quality + test-integration + test-e2e + container-verify + prod-container-verify + migration-test + backup-rehearsal + restart-rehearsal + reconnect-rehearsal + smoke + load-test | All gates PASS; complete release readiness                                |

Reports and traces are written to ignored repository output directories from
inside containers.

## Integration stack

deployment/compose.test.yaml is disposable and uses dedicated database and media
volumes. GEOGUESSME_TEST_WEB_PORT and GEOGUESSME_TEST_MAILPIT_PORT may be set to
non-default ports. The runner derives one public URL and supplies it to
PUBLIC_URL, ALLOWED_ORIGINS, Playwright, WebSocket origins, and email-link
assertions. Mailpit is addressed through the separately derived
MAILPIT_BASE_URL.

The suite covers authentication, group boundaries, challenge lifecycle and media
visibility, transactions, rate limits, storage failures, cleanup retries,
migration concurrency, refresh rotation, WebSocket rejection and cursor
catch-up.

## E2E policy

Every scenario owns and disposes its users, browser contexts, pages, and group
state. Selectors use roles, labels, or stable attributes such as data-photo-id.
Challenge camera mocks are installed with context.addInitScript before page
creation. Camera and geolocation denial contexts assert recoverable UI errors.

E2E style checks reject waitForTimeout, networkidle, positional selectors, and
retry-based flake masking. Accessibility scenarios run real Axe scans and fail
on serious or critical violations.

## Local/CI equivalence

CI checks out the repository, enables Docker Buildx, and invokes the same make
verify target used for local handoff. It does not install Go, Node, Python,
Playwright, or linters on the runner.

The CI workflow uses scoped Docker layer caching (bounded by branch and lockfile
hash), explicit artifact retention of 7 days for failure diagnostics, and
BuildKit GC limits to prevent unbounded cache growth. No secrets or .env files
are cached or uploaded. The check-ci-retention-regression target validates these
properties deterministically.

## Cache and artifact bounds

| Resource              | Scope              | Bound                                                                      |
| --------------------- | ------------------ | -------------------------------------------------------------------------- |
| Docker layer cache    | BuildKit           | `docker builder prune --force` (dangling only); CI: branch+lockfile scoped |
| Named tool caches     | geoguessme-tools   | Removed by make tools-clean or make prune --include-build-cache            |
| Project images        | geoguessme* prefix | prune.sh --max-images 50                                                   |
| Project volumes       | geoguessme* prefix | prune.sh --max-volumes 20 (opt-in)                                         |
| Workspace artifacts   | repo paths         | disk-cleanup.sh --min-age-days 7 --max-total-mb 1024                       |
| CI workflow artifacts | GitHub Actions     | retention-days: 7                                                          |

All cleanup targets are dry-run by default and require explicit CONFIRM before
execution.
