# Deployment

## Repository layout

```
deployment/
├── compose.dev.yaml         # Development stack (PostgreSQL, MinIO, Mailpit, hot reload)
├── compose.test.yaml        # Isolated integration/E2E test stack
├── compose.production.yaml  # Production stack (external services by default)
├── caddy/
│   └── Caddyfile            # Production Caddy configuration (port 80, auto_https off)
├── docker/
│   ├── backend.Dockerfile        # Multi-stage production backend image (distroless)
│   ├── backend.dev.Dockerfile    # Development backend image (Air hot reload)
│   ├── frontend.Dockerfile       # Production frontend gateway (Caddy + SPA)
│   └── frontend.dev.Dockerfile   # Development frontend image (Vite dev server)
├── env/
│   ├── development.env.example
│   ├── production.env.example    # Template for production secrets
│   └── test.env.example
└── scripts/
    ├── backup-postgres.sh        # PostgreSQL backup (gzipped pg_dump)
    ├── restore-postgres.sh       # PostgreSQL restore (interactive confirmation required)
    ├── smoke-test.sh             # HTTP smoke test (liveness, readiness, auth, WS ticket)
    └── wait-for-health.sh        # Block until /health/ready returns 200
```

## Topologies

### Development

Local Docker Compose — PostgreSQL 15, MinIO, Mailpit, backend (Air hot reload),
Vite dev server. See [local-development](local-development.md).

### Test

Ephemeral isolated stack — dedicated volumes, production images, fast
`PHOTO_VIEW_WINDOW=1s`. Treated as disposable. See [testing](testing.md).

### Production

Immutable image tags referenced via environment variables:

| Variable | Description |
|----------|-------------|
| `BACKEND_IMAGE` | Immutable tag (e.g. `ghcr.io/org/geoguessme-backend:1.2.3`) |
| `WEB_IMAGE` | Immutable tag (e.g. `ghcr.io/org/geoguessme-web:1.2.3`) |

External PostgreSQL, SMTP, and S3 by default. Optional Compose profiles
(`local-db`, `local-minio`, `local-smtp`) exist for staging/restore exercises.

Caddy runs on port 80 with `auto_https off`. TLS termination is expected to be
handled by a managed ingress (e.g. Kubernetes Ingress, Cloudflare, nginx) that
proxies to port 80.

## Build and tag

```bash
# Production images
docker build -f deployment/docker/backend.Dockerfile -t registry/geoguessme-backend:1.2.3 .
docker build -f deployment/docker/frontend.Dockerfile -t registry/geoguessme-web:1.2.3 .

make build-images  # Uses Dockerfiles from deployment/docker/
```

The backend image is based on `gcr.io/distroless/static-debian12:nonroot` and
runs as `nonroot:nonroot`. The frontend image uses `caddy:2.10-alpine`.

## Deploy

1. Set `BACKEND_IMAGE`, `WEB_IMAGE`, and create `deployment/env/production.env`
   (see `production.env.example`).

2. Validate configuration:
   ```bash
   make prod-config
   ```

3. Run migrations:
   ```bash
   make prod-migrate
   ```

4. Start the stack:
   ```bash
   make prod-up
   ```

5. Verify:
   ```bash
   make smoke BASE_URL=http://localhost
   ```
   The smoke test checks: liveness (200), readiness (200), protected route
   rejects anonymous (401), WebSocket ticket endpoint rejects anonymous (401).

## HTTP-by-default Caddy

Caddy in the production image has `auto_https off` and listens on `:80`. It is
designed to run behind a managed TLS ingress (e.g. AWS ALB, GCP HTTP(S) LB,
Kubernetes Ingress, Cloudflare Tunnel). If you need Caddy to terminate HTTPS
directly, override the `Caddyfile` or set up a reverse proxy at the edge.

## PostgreSQL upgrade (15 → 16)

1. Stop the stack.
2. Dump the old database:
   ```bash
   pg_dump --format=custom --file=pre-upgrade.dump "$DATABASE_URL_15"
   ```
3. Provision a PostgreSQL 16 instance with an empty database.
4. Restore:
   ```bash
   pg_restore --exit-on-error --dbname="$DATABASE_URL_16" pre-upgrade.dump
   ```
5. Run `make prod-migrate` against the new database.
6. Update `DATABASE_URL` to point to the new instance and start the stack.

## Rollback

Deploy a previous image tag. If a migration has already run, deploy a
compatible previous binary — migrations are forward-only and cannot be reverted
by code. To recover from a bad migration, restore from backup.

## Environment files

`deployment/env/production.env` is git-ignored. The template at
`production.env.example` documents every required variable.

Key production requirements enforced at startup:
- `SMTP_HOST` and `SMTP_FROM` are required
- `SMTP_TLS` must be `starttls` or `tls`
- S3 endpoint must not be localhost
- `JWT_SECRET` must be ≥ 32 characters
- `ALLOWED_ORIGINS` must contain only explicit browser origins (no wildcards)
