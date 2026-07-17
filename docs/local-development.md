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
database and media data across make down and make restart.

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
