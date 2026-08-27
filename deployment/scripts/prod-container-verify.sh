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
SMTP_WEB_PORT="${GEOGUESSME_PROD_VERIFY_SMTP_PORT:-18085}"
export GEOGUESSME_PROD_VERIFY_SMTP_PORT="$SMTP_WEB_PORT"
# The production config requires an HTTPS public origin. The disposable local
# gateway is intentionally plain HTTP, so probes use a separate URL.
PUBLIC_URL="https://localhost:${WEB_PORT}"
PROBE_URL="http://localhost:${WEB_PORT}"

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

# The backend build uses BuildKit's target-platform arguments. Verify that the
# ELF executable agrees with the image manifest so an undeclared TARGETARCH
# cannot silently place an emulated amd64 binary in an arm64 runtime image.
backend_arch="$(docker image inspect --format '{{.Architecture}}' "$backend_image")"
case "$backend_arch" in
    amd64) expected_machine="62 0" ;;
    arm64) expected_machine="183 0" ;;
    *)
        echo "$backend_image has unsupported architecture $backend_arch" >&2
        exit 1
        ;;
esac
binary_machine="$(
    docker run --rm --network none --read-only --cap-drop ALL \
        --security-opt no-new-privileges --entrypoint od "$backend_image" \
        -An -t u1 -j 18 -N 2 /usr/local/bin/geoguessme |
        awk '{$1=$1; print}'
)"
test "$binary_machine" = "$expected_machine" || {
    echo "$backend_image binary architecture mismatch: image=$backend_arch ELF-machine-bytes=$binary_machine" >&2
    exit 1
}
echo "  ok   $backend_image binary architecture=$backend_arch"

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

# Generate a complete test runtime environment. Every variable required by
# the production image is supplied, but APP_ENV=test is intentional: the
# disposable local MinIO fixture only serves plain HTTP. Production-only
# validation (including HTTPS S3 and metrics authentication) is exercised by
# the config tests and real production configuration checks; this rehearsal
# must not weaken those rules just to accommodate a local fixture.
#
# SMTP is configured as an unauthenticated local fixture (Mailpit). No
# credentials are set because Mailpit accepts plain delivery on port 1025.
# SMTP_TLS=starttls keeps the fixture production-compatible, but no emails are
# sent during verification smoke tests so the STARTTLS negotiation with
# Mailpit is never triggered.
cat >"$TMPDIR/production.env" <<'ENVEOF'
APP_ENV=test
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
OIDC_ENABLED=false
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
SMTP_FROM=no-reply@test.local
SMTP_TLS=starttls
SMTP_DIAL_TIMEOUT=10s
SMTP_TIMEOUT=30s
METRICS_TOKEN=test-metrics-token-32-chars-long!!
ENVEOF

# Substitute placeholder values through a second file. BSD and GNU sed use
# incompatible `-i` syntax, while this path behaves identically on both.
sed "s|__PUBLIC_URL__|$PUBLIC_URL|g" "$TMPDIR/production.env" >"$TMPDIR/production.env.rendered"
mv "$TMPDIR/production.env.rendered" "$TMPDIR/production.env"

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
  oauth2-proxy:
    command:
      - --config=/etc/oauth2-proxy/oauth2-proxy.cfg
      - --provider=github
      - --client-id=prod-verify
      - --client-secret=prod-verify-secret
      - --upstream=http://backend:8080/
      - --http-address=0.0.0.0:4180
      - --pass-host-header=true
      - --pass-authorization-header=true
      - --skip-auth-strip-headers=false
      - --cookie-secret=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
      - --redirect-url=https://localhost/oauth2/callback
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
        COMPOSE_PROFILES="local-db,local-minio,local-smtp,social" \
        docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
        --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans 2>/dev/null
    rm -rf "$TMPDIR"
}
trap 'cleanup_stack' EXIT

BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
    COMPOSE_PROFILES="local-db,local-minio,local-smtp,social" \
    docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
    --project-directory "$REPO" -p "$PROJECT" up -d --wait

# ---------------------------------------------------------------------------
# Phase 4: Effective runtime hardening
# ---------------------------------------------------------------------------
echo "--- Phase 4: Effective runtime hardening ---"

container_id() {
    BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
        COMPOSE_PROFILES="local-db,local-minio,local-smtp,social" \
        docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
        --project-directory "$REPO" -p "$PROJECT" ps -aq "$1"
}

assert_inspect() {
    service=$1
    field=$2
    format=$3
    expected=$4
    id=$(container_id "$service")
    test -n "$id" || {
        echo "FAIL: $service container is missing" >&2
        exit 1
    }
    actual=$(docker inspect --format "$format" "$id")
    test "$actual" = "$expected" || {
        echo "FAIL: $service $field is $actual, want $expected" >&2
        exit 1
    }
    echo "  ok   $service $field=$actual"
}

for service in migration backend oauth2-proxy web db minio smtp; do
    assert_inspect "$service" cap_drop '{{join .HostConfig.CapDrop ","}}' ALL
    assert_inspect "$service" no_new_privileges \
        '{{join .HostConfig.SecurityOpt ","}}' no-new-privileges:true
done

for service in migration backend oauth2-proxy web db; do
    assert_inspect "$service" read_only '{{.HostConfig.ReadonlyRootfs}}' true
done

assert_inspect migration pids_limit '{{.HostConfig.PidsLimit}}' 64
assert_inspect backend pids_limit '{{.HostConfig.PidsLimit}}' \
    "${GEOGUESSME_BACKEND_PIDS:-256}"
assert_inspect oauth2-proxy pids_limit '{{.HostConfig.PidsLimit}}' 128
assert_inspect web pids_limit '{{.HostConfig.PidsLimit}}' 128
assert_inspect db pids_limit '{{.HostConfig.PidsLimit}}' 256
assert_inspect minio pids_limit '{{.HostConfig.PidsLimit}}' 128
assert_inspect smtp pids_limit '{{.HostConfig.PidsLimit}}' 128

assert_inspect web cap_add '{{join .HostConfig.CapAdd ","}}' CAP_NET_BIND_SERVICE
assert_inspect db cap_add '{{join .HostConfig.CapAdd ","}}' \
    CAP_CHOWN,CAP_DAC_OVERRIDE,CAP_FOWNER,CAP_SETGID,CAP_SETUID

# The dollar-prefixed names below are Docker's Go-template variables, not shell
# variables.
# shellcheck disable=SC2016
assert_inspect migration networks \
    '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' \
    "${PROJECT}_app "
# shellcheck disable=SC2016
assert_inspect web networks \
    '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' \
    "${PROJECT}_frontend "
for service in db minio smtp; do
    # shellcheck disable=SC2016
    assert_inspect "$service" networks \
        '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' \
        "${PROJECT}_app "
done
# shellcheck disable=SC2016
backend_networks=$(docker inspect --format \
    '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' \
    "$(container_id backend)" | xargs -n1 | sort | xargs)
expected_backend_networks=$(printf '%s\n' "${PROJECT}_app" "${PROJECT}_frontend" | sort | xargs)
test "$backend_networks" = "$expected_backend_networks" || {
    echo "FAIL: backend networks are $backend_networks, want $expected_backend_networks" >&2
    exit 1
}
echo "  ok   backend networks=$backend_networks"

# ---------------------------------------------------------------------------
# Phase 5: Health, readiness, and HTTP verification
# ---------------------------------------------------------------------------
echo "--- Phase 5: Health, readiness, and HTTP verification ---"

# Poll readiness through the gateway.
deadline=$((SECONDS + 120))
ready=0
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -s -o /dev/null -w "%{http_code}" "$PROBE_URL/health/ready" 2>/dev/null || echo 000)
    if [ "$code" = "200" ]; then
        echo "  ok   gateway ready at $PUBLIC_URL"
        ready=1
        break
    fi
    sleep 2
done
if [ "$ready" -eq 0 ]; then
    echo "FAIL: timed out waiting for $PROBE_URL/health/ready" >&2
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

check "liveness" 200 "$PROBE_URL/health/live"
check "readiness" 200 "$PROBE_URL/health/ready"
check "protected route (401)" 401 "$PROBE_URL/api/v1/user/groups"
check "websocket ticket (401)" 401 "$PROBE_URL/api/v1/ws/ticket?group_id=00000000-0000-0000-0000-000000000000"

if [ "$fail" -ne 0 ]; then
    echo "prod-container-verify FAILED: HTTP smoke checks did not pass" >&2
    exit 1
fi

echo ""
echo "prod-container-verify PASSED: non-root users, image healthchecks,"
echo "  effective runtime restrictions, local stack startup, health/readiness,"
echo "  and representative HTTP behavior verified"
