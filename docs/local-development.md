# Local development

## Prerequisites

- Go 1.24+
- Node.js 22+
- Docker and Docker Compose
- `make` (GNU Make)

## Quick start

```bash
# Install all dependencies
make bootstrap

# Start the development stack
make dev
```

This starts the full stack via `deployment/compose.dev.yaml`:

| Service | Image | Exposed port |
|---------|-------|-------------|
| PostgreSQL 15 | `postgres:15-alpine` | `5432` |
| MinIO | `minio/minio:RELEASE.2024-10-13T13-34-11Z` | `9000` (API), `9001` (console) |
| Mailpit | `axllent/mailpit:v1.21` | `1025` (SMTP), `8025` (UI) |
| Backend (hot reload with Air) | Built locally | `8080` |
| Frontend (Vite dev server) | Built locally | `5173` |

## Service URLs

| Service | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| Backend API | http://localhost:8080 |
| MinIO Console | http://localhost:9001 |
| Mailpit | http://localhost:8025 |

## Hot reload

- **Backend**: [Air](https://github.com/air-verse/air) watches `.go` files in
  `backend/` and restarts the server automatically.
- **Frontend**: Vite dev server uses HMR and `CHOKIDAR_USEPOLLING=true` for
  file watching inside the container.

## Common tasks

```bash
make status          # Show container status
make logs            # Tail all logs
make logs-backend    # Tail backend logs only
make logs-frontend   # Tail frontend logs only
make restart         # Restart services
make down            # Stop the stack (volumes are preserved)

# Testing
make test            # Backend unit + frontend unit
make test-backend-race  # Go unit tests with race detector
make test-integration   # Integration tests against isolated stack
make test-e2e           # Playwright desktop + mobile suites
make test-all           # All test suites

# Code quality
make fmt             # Format Go source
make vet             # Run go vet
make lint            # Lint frontend
make ci              # CI-equivalent: fmt-check + vet + test + lint

# Build
make build           # Build frontend bundle + backend binary
```

## Safe shutdown

```bash
make down
```

This stops containers without removing volumes. To reset the development
database and media volumes (destructive):

```bash
make reset-dev CONFIRM=reset-dev
```

## Environment

The development Compose file sets sensible defaults for all environment
variables. You can override by creating a `.env` file at the repository root
(not tracked by git). See [configuration](configuration.md) for every variable.

## Running the backend outside Docker

```bash
cd backend

# Apply migrations
go run . migrate up

# Start the API server
go run . serve
```

The binary also supports `migrate status` and `healthcheck` subcommands.
