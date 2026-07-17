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

make bootstrap builds or pulls the tool images, populates named dependency
caches from the lockfiles, installs the tracked hooks, and runs a tool
self-test. make dev starts PostgreSQL, MinIO, Mailpit, the backend, and the Vite
development server.

## Canonical commands

| Command                               | Purpose                                                           |
| ------------------------------------- | ----------------------------------------------------------------- |
| make bootstrap                        | Prepare the Docker-only toolchain and hooks                       |
| make dev                              | Start the development stack                                       |
| make format / make format-check       | Format or check tracked files                                     |
| make quality                          | Run strict quality, coverage, audit, and build gates              |
| make verify                           | Run the complete integration, E2E, deployment, and rehearsal gate |
| make test-unit / make test-race       | Run unit or race tests                                            |
| make test-integration / make test-e2e | Run isolated live-stack suites                                    |
| make build-images                     | Build production images without cache                             |
| make tools-clean                      | Remove only tool caches and containers                            |

Run make help for the full target list. Hooks use make pre-commit and make
pre-push; they fail when Docker is unavailable and cannot be bypassed.

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
