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
mkcert -install
make dev-social
```

This adds Caddy, local Keycloak, its separate PostgreSQL database, and OAuth2
Proxy. `mkcert -install` is a one-time host setup; the target then generates an
ignored certificate for the local application and identity names. Map
`auth-dev.geoguessme.com` to `127.0.0.1` in `/etc/hosts`, then open
`https://geoguessme.localhost`; Keycloak is at
`https://auth-dev.geoguessme.com`, and verification mail is visible in Mailpit
at `http://localhost:8025`. Caddy is the only browser-facing entrypoint for the
identity path and redirects the matching HTTP names to HTTPS.

Login and signup always display the native Keycloak email path and display the
Google button only when local Google credentials are supplied. After email is
selected, Keycloak owns only the password or registration form; it does not
repeat social providers. Registration and reset mail stays fully local in
Mailpit. Normal pages never render the hidden legacy credential form while OIDC
is enabled.

The checked-in provider values are deliberate placeholders; Keycloak disables
them and the application does not render their buttons. To activate the
downloaded Google Web OAuth client without copying its secret into the
repository, pass its ignored JSON file to the target:

```text
GEOGUESSME_GOOGLE_CLIENT_JSON=/absolute/path/to/client_secret.json make dev-social
```

The target reads the ID and secret with `jq`, enables the Google alias for that
run, and never prints either value. Its local broker callback is
`https://auth-dev.geoguessme.com/realms/geoguessme/broker/google/endpoint`.
Apple and GitHub remain disabled until a later provider rollout.
`make dev-social-down` stops the stack but keeps the named application and
identity volumes.

A genuinely new Google or Keycloak email identity must choose an
empty-by-default GeoGuessMe username after verification. Keycloak's provider
username is never silently copied into the public player profile.

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
