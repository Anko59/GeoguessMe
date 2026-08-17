# Local development

The supported host prerequisites are Git, Make, Docker, and Docker Compose.
Project compilers, package managers, test runners, linters, formatters,
Playwright, and migration tools are available only through Dockerized Make
targets.

## Start

```text
make bootstrap
make dev
```

The development stack runs PostgreSQL, MinIO, Mailpit, a Go backend hot-reload
container, and a Vite frontend container. Named application volumes preserve
database, media, and frontend dependency data across make down and make restart.
The single named frontend dependency volume is reused across rebuilds, so
repeated make dev invocations do not strand anonymous node_modules volumes.
Frontend startup updates that volume from package-lock.json before starting
Vite, keeping dependency changes current without allocating another volume.

## Useful targets

```text
make status
make logs
make logs-backend
make logs-frontend
make restart
make down
make format
make quality
make test-unit
```

Use make reset-dev CONFIRM=reset-dev only when deleting development data is
intentional. make tools-clean removes tool caches and containers without
touching application volumes.

## Configuration

Development defaults are defined in deployment/compose.dev.yaml. Environment
templates live in deployment/env/. Keep secrets out of tracked files. See
configuration.md for the complete variable reference.

Running the backend outside Docker is unsupported. If a new workflow needs a
tool, add a pinned tool image and a Dockerized Make target instead of
documenting a host command.
