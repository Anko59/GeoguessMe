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
bash "$REPO/deployment/oauth2-proxy/prepare-public-configs.sh" "$REPO"

backend_image="${BACKEND_IMAGE:-${LOCAL_BACKEND_IMAGE:?Run through Make or set BACKEND_IMAGE}}"
web_image="${WEB_IMAGE:-${LOCAL_WEB_IMAGE:?Run through Make or set WEB_IMAGE}}"
# Capture immutable IDs once; every inspection and stack start below must use
# these same artifacts even if another build moves either selected local tag.
backend_image="$(docker image inspect --format '{{.Id}}' "$backend_image")"
web_image="$(docker image inspect --format '{{.Id}}' "$web_image")"
if [[ ! "$backend_image" =~ ^sha256:[a-f0-9]{64}$ || ! "$web_image" =~ ^sha256:[a-f0-9]{64}$ ]]; then
    echo 'Docker returned an invalid application image ID' >&2
    exit 2
fi

PROJECT="${GEOGUESSME_PROD_VERIFY_PROJECT:-geoguessme-prod-verify-${GEOGUESSME_TOOLS_PROJECT:?Run through Make}-$$}"
WEB_PORT="${GEOGUESSME_PROD_VERIFY_WEB_PORT:-$((${GEOGUESSME_TEST_PORT_BASE:?Run through Make} + 4))}"
SMTP_WEB_PORT="${GEOGUESSME_PROD_VERIFY_SMTP_PORT:-$((${GEOGUESSME_TEST_PORT_BASE:?Run through Make} + 5))}"
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
ALLOWED_ORIGINS=__PUBLIC_URL__,https://app.geoguessme.com
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
# replace (not merge) the inherited web bindings with one loopback-only port.
# Compose 2.24.4+ supports !override; ordinary ports lists append distinct tuples.
cat >"$TMPDIR/override.yaml" <<YAMLEOF
services:
  migration:
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: true
  backend:
    env_file: !override
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
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: true
  web:
    ports: !override ["127.0.0.1:${WEB_PORT}:80"]
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: false
  db:
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: true
  minio:
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: true
  smtp:
    env_file: !override
      - path: ${TMPDIR}/production.env
        required: false
YAMLEOF

cleanup_stack() {
    local status=$?
    local cleanup_status=0
    BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
        COMPOSE_PROFILES="local-db,local-minio,local-smtp,social" \
        docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
        --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans || cleanup_status=$?
    if [ "$cleanup_status" -ne 0 ]; then
        echo "FAIL: teardown of managed project $PROJECT failed (exit $cleanup_status)" >&2
    fi
    if ! rm -rf "${TMPDIR:?}"; then
        echo "FAIL: removing verification temporary files failed" >&2
        cleanup_status=1
    fi
    # Cleanup must not mask a verification failure, or turn success into a
    # passing gate when managed resources could not be removed.
    if [ "$status" -eq 0 ]; then status=$cleanup_status; fi
    exit "$status"
}
trap 'cleanup_stack' EXIT

# Every service's env_file is replaced above: diagnostics are restricted to
# this managed project with fake fixture values, never an operator's secrets.
fixture_compose() {
    BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
        COMPOSE_PROFILES="local-db,local-minio,local-smtp,social" \
        docker compose -f deployment/compose.production.yaml -f "$TMPDIR/override.yaml" \
        --project-directory "$REPO" -p "$PROJECT" "$@"
}

fixture_compose up -d --wait || {
    status=$?
    echo "FAIL: startup of managed fixture project $PROJECT failed (exit $status)" >&2
    fixture_compose ps --all || echo 'FAIL: fixture status diagnostic failed' >&2
    fixture_compose logs --no-color --tail 100 || echo 'FAIL: fixture logs diagnostic failed' >&2
    ids=$(fixture_compose ps -aq) || ids=""
    for id in $ids; do
        docker inspect --format '{{.Name}} status={{.State.Status}} health={{json .State.Health}}' "$id" ||
            echo 'FAIL: fixture health diagnostic failed' >&2
    done
    exit "$status"
}

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
    shift 3
    code=$(curl -s -D "$TMPDIR/probe-headers" -o /dev/null -w "%{http_code}" "$@" "$url" 2>/dev/null || echo 000)
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

# Inspect only named CORS headers from disposable requests; never print cookie,
# authorization, identity headers, response bodies, or the generated test env.
header_value() {
    awk -v name="$1" 'tolower($1) == tolower(name) ":" {
        sub(/^[^:]+:[[:space:]]*/, ""); sub(/\r$/, ""); print
    }' "$TMPDIR/probe-headers"
}
check_header() {
    if [ "$(header_value "$2")" = "$3" ]; then
        echo "  ok   $1"
    else
        echo "  FAIL $1"
        fail=1
    fi
}
check_header_token() {
    if header_value "$2" | grep -Eq "(^|,)[[:space:]]*$3([[:space:]]*,|$)"; then
        echo "  ok   $1"
    else
        echo "  FAIL $1"
        fail=1
    fi
}

session_url="$PROBE_URL/api/v1/auth/oidc/session"
native_origin=https://app.geoguessme.com
check "OIDC session allowed preflight" 200 "$session_url" -X OPTIONS \
    -H "Origin: $native_origin" -H 'Access-Control-Request-Method: POST' \
    -H 'Access-Control-Request-Headers: Content-Type, Authorization'
check_header "OIDC preflight exact native origin" Access-Control-Allow-Origin "$native_origin"
check_header "OIDC preflight credentials" Access-Control-Allow-Credentials true
check_header_token "OIDC preflight permits POST" Access-Control-Allow-Methods POST
check_header_token "OIDC preflight permits Content-Type" Access-Control-Allow-Headers Content-Type
check_header_token "OIDC preflight permits Authorization" Access-Control-Allow-Headers Authorization
check_header_token "OIDC preflight varies by Origin" Vary Origin
check "OIDC session denied preflight" 403 "$session_url" -X OPTIONS \
    -H 'Origin: https://unapproved.invalid' -H 'Access-Control-Request-Method: POST'
check_header "OIDC denied preflight has no allowed origin" Access-Control-Allow-Origin ''
# The backend's current CORS policy accepts OPTIONS without preflight metadata
# and OPTIONS without Origin. Missing Origin must not gain an allowed origin.
check "OIDC session OPTIONS without preflight headers" 200 "$session_url" -X OPTIONS -H "Origin: $native_origin"
check_header "OIDC metadata-free OPTIONS exact origin" Access-Control-Allow-Origin "$native_origin"
check "OIDC session OPTIONS without Origin" 200 "$session_url" -X OPTIONS -H 'Access-Control-Request-Method: POST'
check_header "OIDC origin-free OPTIONS has no allowed origin" Access-Control-Allow-Origin ''
check "OIDC session bare OPTIONS" 200 "$session_url" -X OPTIONS
check_header "OIDC bare OPTIONS has no allowed origin" Access-Control-Allow-Origin ''
check "OIDC session POST without OAuth cookie" 401 "$session_url" -X POST -H "Origin: $native_origin"
check "OIDC session POST rejects forged identity" 401 "$session_url" -X POST \
    -H "Origin: $native_origin" -H 'Authorization: Bearer forged-test-token' \
    -H 'X-Forwarded-User: forged-user' -H 'X-Forwarded-Email: forged@example.invalid' \
    -H 'X-Forwarded-Preferred-Username: forged-user' -H 'X-Forwarded-Groups: forged-group'

if [ "$fail" -ne 0 ]; then
    echo "prod-container-verify FAILED: HTTP smoke checks did not pass" >&2
    exit 1
fi

echo ""
echo "prod-container-verify PASSED: non-root users, image healthchecks,"
echo "  effective runtime restrictions, local stack startup, health/readiness,"
echo "  and representative HTTP behavior verified"
