# GeoGuessMe deployment

Three isolated Compose topologies — development, test, production — plus the
PostgreSQL 15→16 upgrade path.

---

## Development stack

**Purpose:** hot-reload local gameplay.

**Stack:** PostgreSQL 15 (named volume `geoguessme_dev_db`), MinIO (volume
`geoguessme_dev_minio`), Mailpit, backend with Air hot reload, Vite HMR.

**Volumes (persist across restarts):**

| Volume | Mount | Purpose |
|---|---|---|
| `geoguessme_dev_db` | db:/var/lib/postgresql/data | Database |
| `geoguessme_dev_minio` | minio:/data | S3 media |

**Published ports:**

| Port | Service |
|---|---|
| `5432` | PostgreSQL |
| `9000` | MinIO S3 API |
| `9001` | MinIO console |
| `1025` | Mailpit SMTP |
| `8025` | Mailpit web UI |
| `8080` | Backend API |
| `5173` | Frontend (Vite HMR) |

**Start:**

```bash
make bootstrap   # Go modules + npm ci + Playwright browsers
make dev         # docker compose -f deployment/compose.dev.yaml up -d --build
```

Migration runs automatically on backend startup (`go run . migrate up && air`).

**URLs:**
- Frontend: http://localhost:5173
- Backend API: http://localhost:8080
- MinIO console: http://localhost:9001
- Mailpit: http://localhost:8025

---

## Test stack

**Purpose:** isolated integration and Playwright E2E suites. Never touches
development data.

**Compose project:** `geoguessme-test` (set via `name:`).

**Volumes (ephemeral):** `geoguessme_test_db`, `geoguessme_test_minio` — both
are destroyed when the stack is torn down.

**Published ports:** only two:
- `:8080` → web gateway (serving the production SPA and proxying `/api/*`)
- `:8025` → Mailpit API (used by test assertions, not SMTP)

**Key differences from dev:**
- Production web gateway (`caddy:2.10-alpine`) instead of Vite preview
- Migration runs as a separate one-shot job before backend starts
- Production backend Docker image (distroless, non-root, read-only)
- `PHOTO_VIEW_WINDOW=1s` for fast test assertions
- `RATE_LIMIT_REQUESTS=1000` to avoid rate-limit interference

**Run:**

```bash
# Integration tests (Go):
make test-integration

# Playwright E2E (desktop + mobile):
make test-e2e

# Playwright UI mode:
make test-e2e-ui
```

Both targets tear down the stack (with `-v`) on exit via `trap`.

Secrets are hard-coded in `deployment/compose.test.yaml` for CI convenience.
They are never valid against a real or development database.

---

## Production stack

**Purpose:** real deployment behind a managed TLS ingress.

**Compose project:** `geoguessme-prod`.

**Images (immutable tags, required):**

```bash
export BACKEND_IMAGE=ghcr.io/org/geoguessme-backend:1.2.3
export WEB_IMAGE=ghcr.io/org/geoguessme-web:1.2.3
```

There is no default `latest`; Compose fails fast if either variable is unset.

**Secrets:** `deployment/env/production.env` (git-ignored, copy from
`deployment/env/production.env.example`). Every variable is required; the
backend refuses to start with missing or weak values.

**Services:**

| Service | Default profile | Notes |
|---|---|---|
| `migration` | always | One-shot `BACKEND_IMAGE migrate up`; runs before backend |
| `backend` | always | Non-root, read-only rootfs, writable tmpfs at `/tmp` |
| `web` | always | Caddy 2.10, same-origin gateway, HTTP `:80` |
| `db` | `local-db` only | PostgreSQL 15, named volume `geoguessme_prod_db` |
| `minio` | `local-minio` only | MinIO, volume `geoguessme_prod_minio` |
| `smtp` | `local-smtp` only | Mailpit |

**External services (default):**
- PostgreSQL — supply via `DATABASE_URL` (sslmode=require)
- S3-compatible storage — supply via `S3_*` environment variables
- Authenticated SMTP — supply via `SMTP_*`; TLS mode MUST be `starttls` or `tls`

**Local profiles** (`local-db`, `local-minio`, `local-smtp`) are provided for
staging and restore exercises. Not enabled in a normal deploy.

**Deploy sequence:**

```bash
# 1. Validate config
make prod-config                          # checks BACKEND_IMAGE, WEB_IMAGE, env file

# 2. Run migration (one-shot)
make prod-migrate                         # COMPOSE_PROD run --rm migration migrate up

# 3. Start services
make prod-up                              # COMPOSE_PROD up -d

# 4. Smoke test
make smoke                                # tests /health/live, /health/ready, auth rejection
```

**Migration order:** migration job (`migration migrate up`) must complete
successfully before the backend starts (Compose `depends_on:
condition: service_completed_successfully`).

**Backend hardening:**
- `read_only: true` on the backend container
- `tmpfs: ["/tmp"]` for runtime temp files
- Distroless base image — no shell, no package manager, no `id`
- Health check: `/usr/local/bin/geoguessme healthcheck`
- Non-root user `nonroot:nonroot` (UID 65532)

### HTTPS override (standalone Caddy automatic TLS)

The production gateway listens on `:80` by default because the typical deploy
sits behind a managed TLS ingress (e.g. Cloudflare, AWS ALB, Traefik). To use
Caddy's built-in automatic TLS for a domain, replace the Caddyfile `:80` block
with:

```caddyfile
example.com {
    tls your-email@example.com

    encode gzip zstd

    @api path /api/*
    reverse_proxy @api backend:8080 {
        header_up Host {host}
        flush_interval -1
    }

    header {
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        Referrer-Policy "strict-origin-when-cross-origin"
        Content-Security-Policy "default-src 'self'; connect-src 'self' ws: wss:; img-src 'self' data: blob: https://*.tile.openstreetmap.org; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
    }

    header /assets/* Cache-Control "public, max-age=31536000, immutable"

    root * /srv
    try_files {path} /index.html
    file_server
}
```

This requires the `web` service port mapping changed to `"443:443"` and the
`auto_https off` directive removed from the global options block. Caddy will
obtain and renew Let's Encrypt / ZeroSSL certificates automatically.

---

## PostgreSQL 15 → 16 upgrade

The project uses PostgreSQL 15. To upgrade to PostgreSQL 16:

1. **Back up** production data:
   ```bash
   make db-backup   # uses DATABASE_URL; writes gzipped dump to ./backups/
   ```

2. **Update** the `postgres:15-alpine` image tag to `postgres:16-alpine` in all
   three compose files and rebuild the test stack.

3. **Restore** into the new database:
   ```bash
   make db-restore FILE=backups/geoguessme-20250101T000000Z.sql.gz
   ```
   The restore script (`deployment/scripts/restore-postgres.sh`) is destructive:
   it drops and recreates objects. You must type the database name to confirm.

4. **Verify** with:
   ```bash
   make migrate-status
   make smoke
   ```

**Backup script:** `deployment/scripts/backup-postgres.sh`
**Restore script:** `deployment/scripts/restore-postgres.sh`

Both support `DATABASE_URL` or the classic `PGHOST`/`PGUSER`/`PGDATABASE`/
`PGPASSWORD` variables.

---

## Related documentation

- [Configuration reference](../docs/configuration.md)
- [Operations guide](../docs/operations.md)
- [API reference](../docs/api.md)
- [Testing guide](../docs/testing.md)
