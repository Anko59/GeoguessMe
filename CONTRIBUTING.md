# Contributing

## Prerequisites

- Go 1.24+
- Node.js 22+
- Docker Compose

## Local setup

```bash
make bootstrap   # Go modules + npm ci + Playwright browsers
make dev         # start dev infrastructure (PostgreSQL, MinIO, Mailpit, backend, frontend)
```

## Commit style

Use [Conventional Commits](https://www.conventionalcommits.org/). Examples:

```
feat: add group leaderboard endpoint
fix: validate challenge TTL bounds on startup
docs: explain SMTP TLS modes in deployment README
chore: bump golangci-lint to v1.62
```

Five logical commits are preferred over one large squash.

## Before pushing

Run the full CI check locally:

```bash
make ci
```

This runs `fmt-check`, `vet`, `test-backend`, `test-backend-race`,
`test-frontend`, `lint`, and `audit`.

For broader validation:

```bash
make test-all                 # unit, integration, and E2E
make build-images             # verify Docker images build
```

## Code and docs organisation

- **Backend:** `backend/` — Go HTTP handlers, services, repository, database
  migrations in `backend/internal/database/migrations/`.
- **Frontend:** `frontend/` — React + TypeScript + Vite. Tests with Vitest.
- **Playwright E2E:** `frontend/e2e/`.
- **Integration tests:** `backend/integration_test/`.
- **API docs:** `docs/api.md`.
- **Configuration docs:** `docs/configuration.md`.
- **Deployment and ops:** `deployment/README.md` and `docs/operations.md`.

## Testing guidelines

- Keep authorization checks in services/handlers, not middleware defaults.
- Add a negative test for every new protected operation.
- Use request contexts throughout.
- Do not commit credentials, generated binaries, build output, or uploaded
  media.
