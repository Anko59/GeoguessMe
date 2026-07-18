# Testing

All project test runners execute in Docker. The host invokes Make targets only;
the tool stack supplies Go, Node, Vitest, Playwright, Axe, linters, and coverage
tools from pinned images and named caches.

## Gates

| Target                            | Scope                                                                                         |
| --------------------------------- | --------------------------------------------------------------------------------------------- |
| make test-unit                    | Backend unit tests and frontend Vitest                                                        |
| make test-race                    | Backend race detector                                                                         |
| make test-structure-regression    | Structure-check regression tests in disposable Git repos                                      |
| make test-ci-retention-regression | Verify CI workflow has bounded artifact retention and cache scopes                            |
| make test-cache-status-regression | Cache-status regression tests for Docker resource reporting                                   |
| make test-build-caching           | Verify build-images uses layer caching and clean-build does not                               |
| make cache-status                 | Report project-only Docker images, build cache, volumes, and artifacts (read-only)            |
| make coverage                     | Backend 70% overall and frontend 80/80/80/70 thresholds                                       |
| make test-integration             | Isolated PostgreSQL, MinIO, Mailpit, backend suite                                            |
| make test-e2e                     | Desktop and Pixel 5 Playwright projects                                                       |
| make quality                      | Structure, formatting, linting, type-check, audits, regression tests, tests, coverage, builds |
| make clean-build                  | Rebuild production images without any layer cache                                             |
| make verify                       | Quality plus live-stack, container, migration, backup/restore, restart, smoke, and load gates |

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
hash), explicit artifact retention-days: 7 for failure diagnostics, and BuildKit
GC limits to prevent unbounded cache growth. No secrets or .env files are cached
or uploaded. The check-ci-retention-regression target validates these properties
deterministically.
