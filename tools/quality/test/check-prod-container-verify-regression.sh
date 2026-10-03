#!/usr/bin/env bash
# Regression tests for prod-container-verify.sh.
#
# Tests:
#   1. Script exists and is executable
#   2. Contains all required verification phases
#   3. Handles missing images with clear diagnostic
#   4. Has proper trap/cleanup for teardown
#   5. Enforces non-root user check
#   6. Enforces image healthcheck check
#   7. Validates production compose
#   8. Uses explicit test-only environment values (no production credentials)
#   9. Rejects a backend executable that does not match its image architecture
#  10. Effective Compose ports replace inherited bindings; failed up cleans up
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")/../../.." && pwd)/deployment/scripts/prod-container-verify.sh"
PASS=0
FAIL=0

pass() {
    echo "PASS: $*"
    PASS=$((PASS + 1))
}
fail() {
    echo "FAIL: $*"
    FAIL=$((FAIL + 1))
}

echo "prod-container-verify.sh regression tests:"

# ── Test 1: Script existence and permissions ─────────────────────────────────
echo "--- Test 1: Script existence ---"
if [ -f "$SCRIPT" ]; then pass "script exists"; else fail "script not found at $SCRIPT"; fi
if [ -x "$SCRIPT" ]; then pass "script is executable"; else fail "script is not executable"; fi

# ── Test 2: Required verification phases ─────────────────────────────────────
echo "--- Test 2: Required verification phases ---"
phases=(
    "Image hardening"
    "Compose configuration validation"
    "Start production-like local stack"
    "Effective runtime hardening"
    "Health, readiness, and HTTP verification"
)
for phase in "${phases[@]}"; do
    if grep -q "$phase" "$SCRIPT"; then
        pass "phase marker: '$phase'"
    else
        fail "phase marker missing: '$phase'"
    fi
done

# ── Test 3: Missing image handling ───────────────────────────────────────────
echo "--- Test 3: Missing image diagnostic ---"
if grep -q 'Image.*not found' "$SCRIPT" && grep -q 'make build-images' "$SCRIPT"; then
    pass "diagnostic references 'make build-images' when image is missing"
else
    fail "missing image diagnostic does not reference 'make build-images'"
fi

# ── Test 4: Cleanup trap ─────────────────────────────────────────────────────
echo "--- Test 4: Cleanup trap ---"
if grep -q 'trap.*cleanup_stack.*EXIT' "$SCRIPT"; then
    pass "trap registered for EXIT with cleanup_stack"
else
    fail "no EXIT trap for cleanup_stack"
fi
if grep -q 'down -v --remove-orphans' "$SCRIPT"; then
    pass "cleanup performs docker compose down -v --remove-orphans"
else
    fail "cleanup does not use 'down -v --remove-orphans'"
fi
if grep -q 'rm -rf.*TMPDIR' "$SCRIPT"; then
    pass "cleanup removes temporary directory"
else
    fail "cleanup does not remove temporary directory"
fi

# ── Test 5: Non-root user enforcement ────────────────────────────────────────
echo "--- Test 5: Non-root user enforcement ---"
if grep -q 'runs as root' "$SCRIPT"; then
    pass "explicit root-user rejection message"
else
    fail "no root-user rejection message"
fi
if grep -q 'Config.User' "$SCRIPT"; then
    pass "inspects Config.User for each image"
else
    fail "does not inspect Config.User"
fi

# ── Test 6: Healthcheck enforcement ──────────────────────────────────────────
echo "--- Test 6: Healthcheck enforcement ---"
if grep -q 'Config.Healthcheck' "$SCRIPT"; then
    pass "inspects Config.Healthcheck for each image"
else
    fail "does not inspect Config.Healthcheck"
fi
if grep -q 'no image healthcheck' "$SCRIPT"; then
    pass "explicit missing-healthcheck message"
else
    fail "no missing-healthcheck message"
fi

# ── Test 6b: Backend executable architecture enforcement ────────────────────
echo "--- Test 6b: Backend executable architecture ---"
if grep -q 'Config.*Architecture\|\.Architecture' "$SCRIPT" &&
    grep -q 'binary architecture mismatch' "$SCRIPT"; then
    pass "rejects backend image/executable architecture mismatches"
else
    fail "does not enforce matching backend image and executable architectures"
fi

# ── Test 7: Production compose validation ────────────────────────────────────
echo "--- Test 7: Production compose validation ---"
if grep -q 'compose.production.yaml.*config --quiet' "$SCRIPT"; then
    pass "validates production compose with config --quiet"
else
    fail "does not validate production compose"
fi

# ── Test 8: Test-only environment (no production credentials) ────────────────
echo "--- Test 8: Test-only environment values ---"
# The script must not hardcode production URLs, secrets, or credentials.
prod_indicators=(
    "https://your-domain.example"
    "sslmode=require"
)
for indicator in "${prod_indicators[@]}"; do
    if grep -q "$indicator" "$SCRIPT"; then
        fail "contains production indicator: '$indicator'"
    else
        pass "no production indicator: '$indicator'"
    fi
done
# Verify test-only env markers exist. The local stack deliberately runs the
# application in APP_ENV=test because its disposable MinIO endpoint is HTTP;
# production-only HTTPS validation is covered separately by config tests.
if grep -q '^APP_ENV=test$' "$SCRIPT"; then
    pass "local verification uses APP_ENV=test"
else
    fail "local verification must use APP_ENV=test with HTTP MinIO"
fi
if grep -q 'sslmode=disable' "$SCRIPT"; then
    pass "uses sslmode=disable (test-only)"
else
    fail "missing sslmode=disable (test-only indicator)"
fi
if grep -q 'BCRYPT_COST=4' "$SCRIPT"; then
    pass "uses BCRYPT_COST=4 (test-speed value)"
else
    fail "missing BCRYPT_COST=4"
fi

# ── Test 9: Port configuration ───────────────────────────────────────────────
echo "--- Test 9: Port configuration ---"
if grep -q 'GEOGUESSME_PROD_VERIFY_WEB_PORT' "$SCRIPT"; then
    pass "honors GEOGUESSME_PROD_VERIFY_WEB_PORT override"
else
    fail "does not honor GEOGUESSME_PROD_VERIFY_WEB_PORT override"
fi
if grep -q 'GEOGUESSME_PROD_VERIFY_PROJECT' "$SCRIPT"; then
    pass "honors GEOGUESSME_PROD_VERIFY_PROJECT override"
else
    fail "does not honor GEOGUESSME_PROD_VERIFY_PROJECT override"
fi

# ── Test 10: Smoke check endpoints ───────────────────────────────────────────
echo "--- Test 10: Smoke check endpoints ---"
if grep -q '/health/live' "$SCRIPT" && grep -q '/health/ready' "$SCRIPT"; then
    pass "checks /health/live and /health/ready"
else
    fail "missing liveness/readiness checks"
fi
if grep -q 'api/v1/user/groups' "$SCRIPT"; then
    pass "checks protected route auth enforcement"
else
    fail "missing protected route check"
fi
if grep -q 'api/v1/ws/ticket' "$SCRIPT"; then
    pass "checks websocket ticket auth enforcement"
else
    fail "missing websocket ticket check"
fi

# ── Test 11: COMPOSE_PROFILES for local services ─────────────────────────────
echo "--- Test 11: Local service profiles ---"
if grep -q 'local-db,local-minio,local-smtp' "$SCRIPT"; then
    pass "enables local-db, local-minio, and local-smtp profiles"
else
    fail "does not enable all three local service profiles"
fi

# ── Test 12: Temporary override compose pattern ──────────────────────────────
echo "--- Test 12: Override compose pattern ---"
if grep -q 'override.yaml' "$SCRIPT"; then
    pass "uses compose override file for port and env_file redirection"
else
    fail "does not use compose override file"
fi

# ── Test 13: Container runtime security invariants ───────────────────────────
echo "--- Test 13: Runtime security checks ---"
# Compose schema validation cannot require our security policy, so verify the
# policy markers and the migration dependency explicitly.
COMPOSE_PROD="$(cd "$(dirname "$0")/../../.." && pwd)/deployment/compose.production.yaml"
if [ -f "$COMPOSE_PROD" ]; then
    if grep -q 'read_only: true' "$COMPOSE_PROD"; then
        pass "production compose has read_only: true on backend"
    else
        fail "production compose missing read_only: true"
    fi
    if grep -q 'tmpfs' "$COMPOSE_PROD"; then
        pass "production compose has tmpfs for writable directories"
    else
        fail "production compose missing tmpfs"
    fi
    # Migration must wait for healthy database when local-db profile is active.
    # The depends_on uses required: false so production deploys without local-db
    # are unaffected.
    migration_block=$(awk '
        /^  migration:$/ { inside = 1; next }
        inside && /^  [[:alnum:]_-]+:$/ { exit }
        inside { print }
    ' "$COMPOSE_PROD")
    if grep -q 'condition: service_healthy' <<<"$migration_block"; then
        pass "migration service depends on healthy database"
    else
        fail "migration service missing db health dependency"
    fi
else
    fail "production compose file not found at $COMPOSE_PROD"
fi

# ── Test 14: No contradictory SMTP environment combinations ──────────────────
echo "--- Test 14: No contradictory SMTP environment ---"
# The generated production.env must not set SMTP credentials (USERNAME/PASSWORD)
# while SMTP_TLS=off in production mode — that combination is rejected by the
# backend's production validation ("SMTP_TLS cannot be off in production" and
# "authenticated SMTP requires SMTP_TLS starttls or tls").
#
# Verify the script uses an unauthenticated local SMTP fixture with a non-off
# TLS mode that satisfies production validation.
prod_env_block=$(sed -n '/^cat.*production.env/,/^ENVEOF$/p' "$SCRIPT")

if grep -Eq 'sed[[:space:]]+-i([[:space:]]|$)' "$SCRIPT"; then
    fail "production env substitution uses non-portable sed -i"
else
    pass "production env substitution is portable across GNU and BSD sed"
fi

# SMTP_USERNAME and SMTP_PASSWORD must not appear (unauthenticated fixture).
if grep -qE '^SMTP_USERNAME=' <<<"$prod_env_block"; then
    fail "production.env must not set SMTP_USERNAME (unauthenticated fixture)"
else
    pass "no SMTP_USERNAME in production.env fixture"
fi
if grep -qE '^SMTP_PASSWORD=' <<<"$prod_env_block"; then
    fail "production.env must not set SMTP_PASSWORD (unauthenticated fixture)"
else
    pass "no SMTP_PASSWORD in production.env fixture"
fi

# SMTP_TLS must not be "off" in production (would fail validation).
if grep -qE '^SMTP_TLS=off' <<<"$prod_env_block"; then
    fail "SMTP_TLS=off in production.env would be rejected by production validation"
else
    pass "SMTP_TLS is not off (passes production validation)"
fi

# Verify SMTP_TLS is set to a valid non-off mode.
if grep -qE '^SMTP_TLS=(starttls|tls)' <<<"$prod_env_block"; then
    pass "SMTP_TLS is set to a valid production mode (starttls or tls)"
else
    fail "SMTP_TLS must be starttls or tls for production validation"
fi

# ── Test 15: Immutable image reference pattern in production compose ─────────
echo "--- Test 15: Immutable image references ---"
if [ -f "$COMPOSE_PROD" ]; then
    # shellcheck disable=SC2016  # literal pattern search for compose variable syntax
    if grep -q '${BACKEND_IMAGE:?BACKEND_IMAGE must be set' "$COMPOSE_PROD"; then
        pass "BACKEND_IMAGE uses required immutable reference pattern"
    else
        fail "BACKEND_IMAGE does not use required immutable reference pattern"
    fi
    # shellcheck disable=SC2016  # literal pattern search for compose variable syntax
    if grep -q '${WEB_IMAGE:?WEB_IMAGE must be set' "$COMPOSE_PROD"; then
        pass "WEB_IMAGE uses required immutable reference pattern"
    else
        fail "WEB_IMAGE does not use required immutable reference pattern"
    fi
else
    fail "production compose file not found for image reference check"
fi

# ── Test 16: Effective port merge and failed-start cleanup ────────────────────
echo "--- Test 16: Effective Compose port bindings and lifecycle ---"
# Only real Compose config is allowed through this shim. Image inspection and
# ELF checks are fixtures; up fails deliberately and down never reaches Docker.
# This exercises the script's actual generated override without starting a stack
# or performing operations on production/disposable data.
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
export PROD_VERIFY_REAL_DOCKER
PROD_VERIFY_REAL_DOCKER=$(command -v docker)
cat >"$fixture/docker" <<'DOCKEREOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
    image)
        if [ "$2" != inspect ]; then exit 90; fi
        if [ "${3:-}" != --format ]; then exit 0; fi
        case "$4" in
            '{{.Config.User}}') echo 65532:65532 ;;
            *Healthcheck*) echo '[CMD healthcheck]' ;;
            '{{.Architecture}}') echo amd64 ;;
            *) exit 90 ;;
        esac
        ;;
    run)
        echo '62 0'
        ;;
    inspect)
        test "${4:-}" = fixture-oauth2-proxy
        printf 'health %s\n' "$4" >>"$PROD_VERIFY_CALLS"
        echo 'fixture-oauth2-proxy status=restarting health={"Status":"unhealthy"}'
        ;;
    compose)
        args=("$@")
        override=""
        project=""
        operation=""
        while [ "$#" -gt 0 ]; do
            case "$1" in
                -f)
                    if [[ "$2" == */override.yaml ]]; then override="$2"; fi
                    shift
                    ;;
                -p) project="$2"; shift ;;
                config | up | down | ps | logs) operation="$1" ;;
            esac
            shift
        done
        case "$operation" in
            config)
                exec "$PROD_VERIFY_REAL_DOCKER" "${args[@]}"
                ;;
            up)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                printf 'up %s %s\n' "$project" "$override" >>"$PROD_VERIFY_CALLS"
                # Replace the requested up command with a config-only render.
                config_args=()
                for arg in "${args[@]}"; do
                    if [ "$arg" = up ]; then break; fi
                    config_args+=("$arg")
                done
                "$PROD_VERIFY_REAL_DOCKER" "${config_args[@]}" config >"$PROD_VERIFY_CONFIG"
                exit 73
                ;;
            ps | logs)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                printf '%s %s %s\n' "$operation" "$project" "$override" >>"$PROD_VERIFY_CALLS"
                if [[ " ${args[*]} " == *' ps -aq '* ]]; then
                    echo fixture-oauth2-proxy
                else
                    echo "fixture $operation diagnostic"
                fi
                ;;
            down)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                [[ " ${args[*]} " == *' down -v --remove-orphans '* ]]
                printf 'down %s %s\n' "$project" "$override" >>"$PROD_VERIFY_CALLS"
                if [ "${PROD_VERIFY_DOWN_FAIL:-0}" = 1 ]; then
                    echo 'simulated down failure diagnostic' >&2
                    exit 42
                fi
                ;;
            *) exit 90 ;;
        esac
        ;;
    *) exit 90 ;;
esac
DOCKEREOF
chmod +x "$fixture/docker"

printf 'FORBIDDEN_PRODUCTION_VALUE=fake-private-sentinel\n' >"$fixture/inherited.env"
for scenario in default custom; do
    expected_port=18083
    base_port=8081
    requested_port=""
    if [ "$scenario" = custom ]; then
        expected_port=19083
        base_port=19081
        requested_port="$expected_port"
    fi
    export PROD_VERIFY_CALLS="$fixture/$scenario.calls"
    export PROD_VERIFY_CONFIG="$fixture/$scenario.config"
    project="geoguessme-prod-verify-regression-$scenario"
    status=0
    PATH="$fixture:$PATH" GEOGUESSME_PROD_VERIFY_PROJECT="$project" \
        GEOGUESSME_PROD_VERIFY_WEB_PORT="$requested_port" \
        GEOGUESSME_WEB_PORT="$base_port" \
        GEOGUESSME_ENV_FILE="$fixture/inherited.env" \
        bash "$SCRIPT" >"$fixture/$scenario.log" 2>&1 || status=$?
    if [ "$status" -ne 73 ]; then
        fail "$scenario: expected simulated startup failure (73), got $status"
        cat "$fixture/$scenario.log"
        continue
    fi
    # Compose's normalized YAML exposes every effective binding, rather than
    # merely checking that the source override contains the desired port.
    bindings=$(awk '
        /^  web:$/ { web = 1; next }
        web && /^  [[:alnum:]_-]+:$/ { exit }
        web && /^    ports:$/ { ports = 1; next }
        ports && /^    [[:alnum:]_-]+:/ { ports = 0 }
        ports && /host_ip:|target:|published:|protocol:/ {
            gsub(/"/, ""); print $1, $2
        }
    ' "$PROD_VERIFY_CONFIG")
    expected=$(printf 'host_ip: 127.0.0.1\ntarget: 80\npublished: %s\nprotocol: tcp' "$expected_port")
    if [ "$bindings" = "$expected" ]; then
        pass "$scenario: exactly one loopback gateway binding, replaces inherited $base_port"
    else
        fail "$scenario: effective gateway bindings differ from requested loopback-only port"
        printf '%s\n' "$bindings"
    fi
    override=$(awk '$1 == "up" { print $3 }' "$PROD_VERIFY_CALLS")
    expected_calls=$(printf 'up %s %s\nps %s %s\nlogs %s %s\nps %s %s\nhealth fixture-oauth2-proxy\ndown %s %s' \
        "$project" "$override" "$project" "$override" "$project" "$override" \
        "$project" "$override" "$project" "$override")
    if grep -q 'fixture logs diagnostic' "$fixture/$scenario.log" &&
        grep -q 'health={"Status":"unhealthy"}' "$fixture/$scenario.log" &&
        ! grep -q 'FORBIDDEN_PRODUCTION_VALUE' "$PROD_VERIFY_CONFIG"; then
        pass "$scenario: fixture-only logs/status/health emitted before teardown"
    else
        fail "$scenario: diagnostic missing or inherited env_file retained"
    fi
    if [ "$(<"$PROD_VERIFY_CALLS")" = "$expected_calls" ] &&
        [ -n "$override" ] && [ ! -e "$(dirname "$override")" ]; then
        pass "$scenario: failed up tears down only managed project and removes temp files"
    else
        fail "$scenario: failed-start lifecycle/temporary-file cleanup differs"
    fi
done

# shellcheck source=tools/quality/test/prod-container-verify/cleanup-regression.sh
source "$(dirname "$0")/prod-container-verify/cleanup-regression.sh"

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "$FAIL" -eq 0 ]; then
    echo "prod-container-verify.sh regression tests PASSED"
else
    echo "prod-container-verify.sh regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
