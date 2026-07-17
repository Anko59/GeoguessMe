# GeoGuessMe

Real-time multiplayer photo location guessing game. Group members share
short-lived private photo challenges, view each photo for a timed window, and
submit one server-timed guess. Closest guess wins.

**Status:** active development.

## Architecture

One-page React app talks to a Go HTTP/WebSocket API behind a same-origin Caddy
gateway. Media is uploaded to a private S3-compatible bucket and streamed
through the authenticated backend — no direct browser-to-S3 access.

- **Frontend:** React + TypeScript + Vite, served by Caddy.
- **Backend:** Go 1.24 HTTP/WebSocket API with embedded ordered SQL migrations.
- **Database:** PostgreSQL 15 with session-advisory-lock migrations.
- **Media storage:** Private S3-compatible bucket (MinIO in development).
- **Email:** Authenticated SMTP with `starttls`/`tls` policy (Mailpit in
  development).
- **Gateway:** Caddy 2.10, same-origin proxy of `/api/*` including
  `/api/v1/ws`. No public uploads directory.

## Prerequisites

- Go 1.24
- Node.js 22
- Docker Compose (for dev/test infrastructure)

## Quick start

```bash
make bootstrap    # install Go modules, npm packages, Playwright browsers
make dev          # start PostgreSQL, MinIO, Mailpit, backend, frontend
```

The development stack starts PostgreSQL, MinIO, Mailpit, the Go backend (with
[Air](https://github.com/air-verse/air) hot reload), and Vite HMR.

**Dev URLs:**

| Service | URL |
|---|---|
| Frontend | http://localhost:5173 |
| Backend API | http://localhost:8080 |
| MinIO console | http://localhost:9001 |
| Mailpit | http://localhost:8025 |

## Important Make commands

| Command | Purpose |
|---|---|
| `make dev` / `make up` | Start development stack |
| `make down` | Stop development (keeps volumes) |
| `make reset-dev CONFIRM=reset-dev` | Delete development volumes |
| `make logs` | Tail all dev logs |
| `make fmt-check` | Fail on unformatted Go |
| `make test` | Backend unit + frontend unit |
| `make test-backend-race` | Go tests with race detector |
| `make test-integration` | Backend integration against isolated stack |
| `make test-e2e` | Playwright desktop + mobile |
| `make test-all` | Unit, integration, and E2E suites |
| `make ci` | Run the same fast checks as CI |
| `make build-images` | Build production Docker images |
| `make migrate-status` | Show applied/pending SQL migrations |
| `make db-backup` | Gzipped pg_dump |
| `make db-restore FILE=...` | Destructive restore with confirmation |
| `make smoke` | Smoke test a running gateway |

## Testing

Fast checks: `make ci` (fmt, vet, race unit tests, lint, audit).

Integration: `make test-integration` starts an isolated Compose project with
production images, runs the Go integration suite, and tears down (`-v`).

E2E: `make test-e2e` runs real Playwright desktop and mobile scenarios against
the same isolated stack.

See [docs/testing.md](docs/testing.md) for details.

## Production deployment

Build the two immutable Docker images and deploy via the production Compose
file:

```bash
export BACKEND_IMAGE=ghcr.io/org/geoguessme-backend:1.2.3
export WEB_IMAGE=ghcr.io/org/geoguessme-web:1.2.3
make prod-migrate && make prod-up
```

Full deployment guide: [deployment/README.md](deployment/README.md).
Operations (backup, restore, incident response): [docs/operations.md](docs/operations.md).

## Documentation

Full documentation lives in [`docs/`](docs/index.md) — start at the [documentation index](docs/index.md).

- [API reference](docs/api.md)
- [Configuration](docs/configuration.md)
- [Deployment](deployment/README.md)
- [Testing](docs/testing.md)
- [Operations](docs/operations.md)

## Policies

- [Security](SECURITY.md) — reporting vulnerabilities
- [Privacy](PRIVACY.md) — data inventory, retention, account deletion
- [Contributing](CONTRIBUTING.md) — setup, commit style, CI
- [License](LICENSE)
