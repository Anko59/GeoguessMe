#!/usr/bin/env bash
set -euo pipefail

# Production-container verification: build pinned images, validate non-root /
# healthcheck / read-only / compose invariants, start a local production-like
# stack with explicit test env, poll health/readiness, verify representative
# HTTP behavior, then tear down all project resources.
#
# Required host prerequisites: Docker, Docker Compose.
# No production credentials are required or invented; local-db, local-minio, and
# local-smtp profiles supply disposable test services.

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

backend_image="${BACKEND_IMAGE:-geoguessme-backend:local}"
web_image="${WEB_IMAGE:-geoguessme-web:local}"

PROJECT="${GEOGUESSME_PROD_VERIFY_PROJECT:-geoguessme-prod-verify}"
WEB_PORT="${GEOGUESSME_PROD_VERIFY_WEB_PORT:-18083}"
PUBLIC_URL="http://localhost:${WEB_PORT}"

# ---------------------------------------------------------------------------
# Phase 1: Image hardening checks
# ---------------------------------------------------------------------------
echo "--- Phase 1: Image hardening checks ---"

for image in "$backend_image" "$web_image"; do
    docker image inspect "$image" >/dev/null || {
        echo "Image $image not found. Run: make build-images" >&2
        exit 2
    }
    user="$(docker image inspect --format '{{.Config.User}}' "$image")"
    test -n "$user" || {
        echo "$image has no explicit non-root user" >&2
        exit 1
    }
    case "$user" in
        0 | root | 0:0 | root:root)
            echo "$image runs as root" >&2
            exit 1
            ;;
    esac
    echo "  ok   $image user=$user"

    health="$(docker image inspect --format '{{if .Config.Healthcheck}}{{.Config.Healthcheck.Test}}{{end}}' "$image")"
    test -n "$health" || {
        echo "$image has no image healthcheck" >&2
        exit 1
    }
    echo "  ok   $image healthcheck=$health"
done

# ---------------------------------------------------------------------------
# Phase 2: Validate production Compose configuration
# ---------------------------------------------------------------------------
echo "--- Phase 2: Compose configuration validation ---"

BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
    docker compose -f deployment/compose.production.yaml --project-directory . config --quiet
echo "  ok   production compose validates"

# ---------------------------------------------------------------------------
# Phase 3: Create temporary test environment and start stack
# ---------------------------------------------------------------------------
echo "--- Phase 3: Start production-like local stack ---"

TMPDIR="$(mktemp -d)"

# Generate a complete test production environment. Every variable required by
# the production backend is supplied. Values are explicitly local/test;
# production credentials are never invented.
cat >"$TMPDIR/production.env" <<'ENVEOF'
APP_ENV=production
PORT=8080
PUBLIC_URL=__PUBLIC_URL__
LOG_LEVEL=info
DATABASE_URL=postgres://test:test@db:5432/geoguessme?sslmode=disable
POSTGRES_USER=test
POSTGRES_PASSWORD=test
POSTGRES_DB=geoguessme
DB_MIN_CONNS=2
DB_MAX_CONNS=10
JWT_SECRET=test-secret-key-at-least-32-chars-long-prod-verify
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
VERIFICATION_TOKEN_TTL=24h
RESET_TOKEN_TTL=1h
BCRYPT_COST=4
ALLOWED_ORIGINS=__PUBLIC_URL__
TRUSTED_PROXY_CIDRS=0.0.0.0/0
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=1m
S3_ENDPOINT=http://minio:9000
S3_REGION=us-east-1
S3_BUCKET=geoguessme-prod-verify
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_USE_PATH_STYLE=true
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
UPLOAD_MAX_BYTES=5242880
UPLOAD_MAX_PIXELS=25000000
CHALLENGE_TTL=24h
PHOTO_VIEW_WINDOW=10s
PHOTO_RETENTION=720h
SMTP_HOST=smtp
SMTP_PORT=1025
SMTP_USERNAME=test
SMTP_PASSWORD=test
SMTP_FROM=no-reply@test.local
SMTP_TLS=off
SMTP_DIAL_TIMEOUT=10s
SMTP_TIMEOUT=30s
METRICS_TOKEN=test-metrics-token-32-chars-long!!
ENVEOF

# Substitute placeholder values.
sed -i "s|__PUBLIC_URL__|$PUBLIC_URL|g" "$TMPDIR/production.env"

# Compose override: redirect env_file to the temp file for every service and
# override the web port to avoid host port conflicts.
cat >"$TMPDIR/override.yaml" <<YAMLEOF
services:
  migration:
    env_file:
      - path: ${TMPDIR}/production.env
        required: true
  backend:
    env_file:
      - path: ${TMPDIR}/production.env
        required: true
  web:
    ports: ["${WEB_PORT}:80"]
    env_file:
      - path: ${TMPDIR}/production.env
        required: false
  db:
    env_file:
      - path: ${TMPDIR}/production.env
        required: true
  minio:
    env_file:
      - path: ${TMPDIR}/production.env
        required: true
  smtp:
    env_file:
      - path: ${TMPDIR}/production.env
        required: false
YAMLEOF

cleanup_stack() {
    set +e
    BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
        COMPOSE_PROFILES="local-db,local-minio,local-smtp" \
        docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
        --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans 2>/dev/null
    rm -rf "$TMPDIR"
}
trap 'cleanup_stack' EXIT

BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
    COMPOSE_PROFILES="local-db,local-minio,local-smtp" \
    docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
    --project-directory "$REPO" -p "$PROJECT" up -d --wait

# ---------------------------------------------------------------------------
# Phase 4: Health, readiness, and HTTP verification
# ---------------------------------------------------------------------------
echo "--- Phase 4: Health, readiness, and HTTP verification ---"

# Poll readiness through the gateway.
deadline=$((SECONDS + 120))
ready=0
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -s -o /dev/null -w "%{http_code}" "$PUBLIC_URL/health/ready" 2>/dev/null || echo 000)
    if [ "$code" = "200" ]; then
        echo "  ok   gateway ready at $PUBLIC_URL"
        ready=1
        break
    fi
    sleep 2
done
if [ "$ready" -eq 0 ]; then
    echo "FAIL: timed out waiting for $PUBLIC_URL/health/ready" >&2
    exit 1
fi

# Smoke checks: liveness, readiness, auth enforcement, WebSocket auth.
fail=0
check() {
    desc="$1"
    expected="$2"
    url="$3"
    code=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo 000)
    if [ "$code" = "$expected" ]; then
        echo "  ok   $desc ($code)"
    else
        echo "  FAIL $desc (got $code, want $expected)"
        fail=1
    fi
}

check "liveness" 200 "$PUBLIC_URL/health/live"
check "readiness" 200 "$PUBLIC_URL/health/ready"
check "protected route (401)" 401 "$PUBLIC_URL/api/v1/user/groups"
check "websocket ticket (401)" 401 "$PUBLIC_URL/api/v1/ws/ticket?group_id=00000000-0000-0000-0000-000000000000"

if [ "$fail" -ne 0 ]; then
    echo "prod-container-verify FAILED: HTTP smoke checks did not pass" >&2
    exit 1
fi

echo ""
echo "prod-container-verify PASSED: non-root users, image healthchecks,"
echo "  production Compose validation, local stack startup, health/readiness,"
echo "  and representative HTTP behavior verified"
