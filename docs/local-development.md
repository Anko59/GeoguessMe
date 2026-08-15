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
database and media data across make down and make restart. Each make dev rebuild
renews the frontend's anonymous dependency volume so package-lock changes are
available to Vite without deleting persistent application data.

To launch the complete identity path instead, use:

```text
make dev-social
```

This adds local Keycloak, its separate PostgreSQL database, and OAuth2 Proxy.
Open `http://geoguessme.localhost:5173`; Keycloak is at
`http://auth.geoguessme.localhost:8083`. The imported realm displays Google,
Apple, and GitHub with placeholder provider credentials, so provider buttons are
visually testable while a local Keycloak user exercises the complete app
signup/login flow. Normal `/login` and `/signup` contain no application password
fields in this mode. `make dev-social-down` stops the stack but keeps the named
application and identity volumes.

## Useful targets

```text
make status
make logs
make logs-backend
make logs-frontend
make restart
make dev-social
make dev-social-down
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
