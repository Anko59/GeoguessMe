# Testing

## Test pyramid

```
    ╱    E2E (Playwright desktop + mobile)    ╲
   ╱          Integration (Go + live stack)      ╲
  ╱           Unit (Go + Vitest)                    ╲
```

### Unit

- **Backend**: `go test ./...` (excludes `integration_test/`). Package tests
  with `testing.T` and `testify/assert`. Race detection with `-race`.
- **Frontend**: `npm test -- --run` (Vitest). Component and hook tests.

### Integration

- **Backend integration**: Tests in `backend/integration_test/` run against an
  isolated live stack managed by `deployment/compose.test.yaml`.
- Run via `make test-integration`.
- Tests cover: signup, login, token refresh, group create/join, photo upload,
  challenge accept, guess submission, result visibility, WebSocket messaging,
  cursor pagination, rate limiting, and negative cases (non-member access,
  expired challenges, duplicate guesses).

### E2E

- Playwright with two projects: `desktop` (Chromium) and `mobile` (Pixel).
- Run via `make test-e2e` or `make test-e2e-ui` for the Playwright UI mode.
- Tests start the test stack, wait for readiness, execute suites, then tear
  down.

## Make targets

| Target | Description |
|--------|-------------|
| `test` | Backend unit + frontend unit |
| `test-backend` | Go unit tests (excludes integration) |
| `test-backend-race` | Go unit tests with race detector |
| `test-frontend` | Frontend unit tests (Vitest) |
| `test-integration` | Backend integration tests against isolated stack |
| `test-e2e` | Playwright desktop + mobile against isolated stack |
| `test-e2e-ui` | Playwright UI mode against isolated stack |
| `test-all` | All test suites |
| `coverage` | Go coverage report (func summary) |

## Isolated test stack

`deployment/compose.test.yaml` provides:

- PostgreSQL 15 (dedicated volume)
- MinIO (dedicated volume)
- Mailpit
- Migration job (runs `migrate up`)
- Backend (production image, `PHOTO_VIEW_WINDOW=1s`, `RATE_LIMIT_REQUESTS=1000`)
- Web / Caddy gateway (port 8080)

The stack uses the `geoguessme-test` Compose project name and dedicated volumes
so it never touches development data. It is torn down automatically after each
run (`down -v`).

## Playwright configuration

The frontend has `playwright.config.ts` defining projects:

- **desktop**: Chromium, viewport 1280×720
- **mobile**: Pixel 5, viewport 393×851

`PLAYWRIGHT_BASE_URL` is set to the test stack gateway URL.

## Debugging

- `make test-e2e-ui` launches the Playwright UI mode with trace viewer.
- The test stack stays up across runs when invoked manually.
- Screenshots, traces, and test results are written to `frontend/test-results/`
  and `frontend/playwright-report/`.

## CI equivalence

`make ci` runs the same checks as the CI pipeline:

1. `fmt-check` — Go formatting (vet-style, no changes made)
2. `vet` — `go vet`
3. `test-backend` — Go unit tests
4. `test-backend-race` — Go unit tests with race detector
5. `test-frontend` — Frontend unit tests
6. `lint` — Frontend lint
7. `audit` — Go dependency vulnerability scan

## Reports

- Go coverage: `coverage.out` in the `backend/` directory.
- Playwright report: `frontend/playwright-report/`.
- Blob report: `frontend/blob-report/`.

## CI release gate

The release gate additionally:

1. Builds production Docker images (`backend.Dockerfile`, `frontend.Dockerfile`)
2. Checks non-root container startup
3. Runs the health smoke test (`make smoke`)
4. Verifies backup/restore in a separate database
