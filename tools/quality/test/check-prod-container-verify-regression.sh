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
# Verify test-only env markers exist.
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
# The script must verify the production compose has read_only and healthcheck
# settings, but these are validated structurally via compose validation.
# The script already validates compose config --quiet which catches missing
# service-level properties. Verify the compose production file still has them.
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

# SMTP_USERNAME and SMTP_PASSWORD must not appear (unauthenticated fixture).
if echo "$prod_env_block" | grep -qE '^SMTP_USERNAME='; then
    fail "production.env must not set SMTP_USERNAME (unauthenticated fixture)"
else
    pass "no SMTP_USERNAME in production.env fixture"
fi
if echo "$prod_env_block" | grep -qE '^SMTP_PASSWORD='; then
    fail "production.env must not set SMTP_PASSWORD (unauthenticated fixture)"
else
    pass "no SMTP_PASSWORD in production.env fixture"
fi

# SMTP_TLS must not be "off" in production (would fail validation).
if echo "$prod_env_block" | grep -qE '^SMTP_TLS=off'; then
    fail "SMTP_TLS=off in production.env would be rejected by production validation"
else
    pass "SMTP_TLS is not off (passes production validation)"
fi

# Verify SMTP_TLS is set to a valid non-off mode.
if echo "$prod_env_block" | grep -qE '^SMTP_TLS=(starttls|tls)'; then
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

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "$FAIL" -eq 0 ]; then
    echo "prod-container-verify.sh regression tests PASSED"
else
    echo "prod-container-verify.sh regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
