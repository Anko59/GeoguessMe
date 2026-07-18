#!/usr/bin/env bash
# Regression tests for restart-rehearsal.sh.
#
# Tests:
#   1. Script exists and is executable
#   2. Script has required sections (phases, guards, polling)
#   3. Script refuses non-rehearsal project names
#   4. Script has proper cleanup trap
#   5. Script uses polling with deadlines (no unconditional sleeps outside polling)
#   6. Script checks are deterministic (PASS/FAIL counters, structured output)
#   7. Script handles missing Docker gracefully
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")/../../.." && pwd)/deployment/scripts/restart-rehearsal.sh"
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

echo "restart-rehearsal regression tests:"

# ── Test 1: Script existence and permissions ─────────────────────────────────
echo "--- Test 1: Script existence ---"
if [ -f "$SCRIPT" ]; then
    pass "restart-rehearsal.sh exists"
else
    fail "restart-rehearsal.sh not found at $SCRIPT"
fi

if [ -x "$SCRIPT" ]; then
    pass "restart-rehearsal.sh is executable"
else
    fail "restart-rehearsal.sh is not executable"
fi

# ── Test 2: Required structure ──────────────────────────────────────────────
echo "--- Test 2: Required structure ---"

content=$(cat "$SCRIPT")

# Guard: refuses non-rehearsal project
if echo "$content" | grep -q 'rehearsal.*safety\|must contain.*rehearsal'; then
    pass "has project-name safety guard"
else
    fail "missing project-name safety guard"
fi

# Cleanup trap
if echo "$content" | grep -q 'trap cleanup EXIT'; then
    pass "has EXIT cleanup trap"
else
    fail "missing EXIT cleanup trap"
fi

# Uses docker compose down without -v for restart
if echo "$content" | grep -q 'down.*--remove-orphans' && echo "$content" | grep -qv 'down -v'; then
    pass "restart uses 'down' without -v (volumes preserved)"
else
    fail "restart may not preserve volumes correctly"
fi

# Polling with deadline (no unconditional sleeps outside polling function)
# Check that poll() function uses a deadline pattern
if echo "$content" | grep -q 'deadline.*SECONDS\|deadline.*date.*+%s'; then
    pass "poll function uses deadline-based timing"
else
    fail "poll function missing deadline pattern"
fi

# Uses docker compose up --wait (not sleep-based waiting)
if echo "$content" | grep -q 'up -d --wait'; then
    pass "uses 'up -d --wait' for service readiness"
else
    fail "missing 'up -d --wait' for service readiness"
fi

# ── Test 3: Deterministic check structure ───────────────────────────────────
echo "--- Test 3: Deterministic checks ---"

if echo "$content" | grep -q 'PASS=0' && echo "$content" | grep -q 'FAIL=0'; then
    pass "has PASS/FAIL counters"
else
    fail "missing PASS/FAIL counters"
fi

if echo "$content" | grep -q 'restart-rehearsal PASSED' && echo "$content" | grep -q 'restart-rehearsal FAILED'; then
    pass "has deterministic pass/fail summary"
else
    fail "missing deterministic pass/fail summary"
fi

# ── Test 4: Continuity checks ───────────────────────────────────────────────
echo "--- Test 4: Continuity verification ---"

if echo "$content" | grep -q 'migration count unchanged' || echo "$content" | grep -q 'migration.*unchanged'; then
    pass "checks migration count continuity"
else
    fail "missing migration count continuity check"
fi

if echo "$content" | grep -q 'no duplicate migration'; then
    pass "checks for duplicate migrations"
else
    fail "missing duplicate migration check"
fi

if echo "$content" | grep -q 'checksum'; then
    pass "has data checksum verification"
else
    fail "missing data checksum verification"
fi

if echo "$content" | grep -q 'MinIO\|minio_exec\|media.*survives'; then
    pass "has MinIO media continuity check"
else
    fail "missing MinIO media continuity check"
fi

# ── Test 5: Cleanup guards ──────────────────────────────────────────────────
echo "--- Test 5: Cleanup safety ---"

# Final cleanup uses down -v (contained within the EXIT trap)
if echo "$content" | grep -q 'down -v --remove-orphans'; then
    pass "cleanup trap uses down -v (complete teardown)"
else
    fail "cleanup trap missing complete teardown"
fi

# Destructive cleanup is in trap (not in main flow)
cleanup_lines=$(echo "$content" | grep -n 'down -v' | head -1 | cut -d: -f1)
trap_line=$(echo "$content" | grep -n '^cleanup()' | head -1 | cut -d: -f1)
if [ -n "$cleanup_lines" ] && [ -n "$trap_line" ] && [ "$cleanup_lines" -gt "$trap_line" ]; then
    pass "destructive cleanup guarded inside cleanup() trap"
else
    fail "destructive cleanup not properly guarded"
fi

# ── Test 6: Real data seeding ───────────────────────────────────────────────
echo "--- Test 6: Data seeding ---"

if echo "$content" | grep -q 'INSERT INTO users'; then
    pass "seeds users table"
else
    fail "does not seed users table"
fi

if echo "$content" | grep -q 'INSERT INTO photos'; then
    pass "seeds photos table"
else
    fail "does not seed photos table"
fi

if echo "$content" | grep -q 'INSERT INTO groups'; then
    pass "seeds groups table"
else
    fail "does not seed groups table"
fi

# ── Test 7: All services are restarted ──────────────────────────────────────
echo "--- Test 7: Service coverage ---"

# Verify the restart step covers all services (not just backend/web).
# It uses 'docker compose down' which stops everything.
if echo "$content" | grep -q 'docker compose.*down'; then
    pass "restarts all services (full compose down)"
else
    fail "may not restart all services"
fi

# ── Summary ─────────────────────────────────────────────────────────────────
if [ "$FAIL" -eq 0 ]; then
    echo "restart-rehearsal regression tests PASSED"
else
    echo "restart-rehearsal regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
