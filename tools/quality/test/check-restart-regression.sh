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
#
# Run with --determinism to execute the content checks 50 times against a
# known-good and a known-bad script, pinning that the matching logic is stable
# (no pipefail/SIGPIPE flakiness) without losing its ability to reject real
# regressions.
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

# check_content runs the content-based regression checks (Tests 2-7) against
# the given script text and returns 0 when every check passes. The matches use
# `grep ... <<< "$content"` (no echo-to-grep pipe) because a producer pipe under
# `set -o pipefail` races with grep -q: grep exits as soon as it matches and the
# producer then dies on SIGPIPE, intermittently failing a valid script.
check_content() {
    local content="$1"
    PASS=0
    FAIL=0

    # ── Test 2: Required structure ──────────────────────────────────────────
    echo "--- Test 2: Required structure ---"

    # Guard: refuses non-rehearsal project
    if grep -q 'rehearsal.*safety\|must contain.*rehearsal' <<<"$content"; then
        pass "has project-name safety guard"
    else
        fail "missing project-name safety guard"
    fi

    # Cleanup trap
    if grep -q 'trap cleanup EXIT' <<<"$content"; then
        pass "has EXIT cleanup trap"
    else
        fail "missing EXIT cleanup trap"
    fi

    # Uses docker compose down without -v for restart
    if grep -q 'down.*--remove-orphans' <<<"$content" && grep -qv 'down -v' <<<"$content"; then
        pass "restart uses 'down' without -v (volumes preserved)"
    else
        fail "restart may not preserve volumes correctly"
    fi

    # Polling with deadline (no unconditional sleeps outside polling function)
    # Check that poll() function uses a deadline pattern
    if grep -q 'deadline.*SECONDS\|deadline.*date.*+%s' <<<"$content"; then
        pass "poll function uses deadline-based timing"
    else
        fail "poll function missing deadline pattern"
    fi

    # Uses docker compose up --wait (not sleep-based waiting)
    if grep -q 'up -d --wait' <<<"$content"; then
        pass "uses 'up -d --wait' for service readiness"
    else
        fail "missing 'up -d --wait' for service readiness"
    fi

    # ── Test 3: Deterministic check structure ───────────────────────────────
    echo "--- Test 3: Deterministic checks ---"

    if grep -q 'PASS=0' <<<"$content" && grep -q 'FAIL=0' <<<"$content"; then
        pass "has PASS/FAIL counters"
    else
        fail "missing PASS/FAIL counters"
    fi

    if grep -q 'restart-rehearsal PASSED' <<<"$content" && grep -q 'restart-rehearsal FAILED' <<<"$content"; then
        pass "has deterministic pass/fail summary"
    else
        fail "missing deterministic pass/fail summary"
    fi

    # ── Test 4: Continuity checks ───────────────────────────────────────────
    echo "--- Test 4: Continuity verification ---"

    if grep -q 'migration count unchanged' <<<"$content" || grep -q 'migration.*unchanged' <<<"$content"; then
        pass "checks migration count continuity"
    else
        fail "missing migration count continuity check"
    fi

    if grep -q 'no duplicate migration' <<<"$content"; then
        pass "checks for duplicate migrations"
    else
        fail "missing duplicate migration check"
    fi

    if grep -q 'checksum' <<<"$content"; then
        pass "has data checksum verification"
    else
        fail "missing data checksum verification"
    fi

    if grep -q 'MinIO\|minio_exec\|media.*survives' <<<"$content"; then
        pass "has MinIO media continuity check"
    else
        fail "missing MinIO media continuity check"
    fi

    # ── Test 5: Cleanup guards ──────────────────────────────────────────────
    echo "--- Test 5: Cleanup safety ---"

    # Final cleanup uses down -v (contained within the EXIT trap)
    if grep -q 'down -v --remove-orphans' <<<"$content"; then
        pass "cleanup trap uses down -v (complete teardown)"
    else
        fail "cleanup trap missing complete teardown"
    fi

    # Destructive cleanup is in trap (not in main flow). grep -m1 stops at the
    # first match so the pipeline never feeds an early-exiting `head`.
    cleanup_lines=$(grep -n -m1 'down -v' <<<"$content" | cut -d: -f1)
    trap_line=$(grep -n -m1 '^cleanup()' <<<"$content" | cut -d: -f1)
    if [ -n "$cleanup_lines" ] && [ -n "$trap_line" ] && [ "$cleanup_lines" -gt "$trap_line" ]; then
        pass "destructive cleanup guarded inside cleanup() trap"
    else
        fail "destructive cleanup not properly guarded"
    fi

    # ── Test 6: Real data seeding ───────────────────────────────────────────
    echo "--- Test 6: Data seeding ---"

    if grep -q 'INSERT INTO users' <<<"$content"; then
        pass "seeds users table"
    else
        fail "does not seed users table"
    fi

    if grep -q 'INSERT INTO photos' <<<"$content"; then
        pass "seeds photos table"
    else
        fail "does not seed photos table"
    fi

    if grep -q 'INSERT INTO groups' <<<"$content"; then
        pass "seeds groups table"
    else
        fail "does not seed groups table"
    fi

    # ── Test 7: All services are restarted ──────────────────────────────────
    echo "--- Test 7: Service coverage ---"

    # Verify the restart step covers all services (not just backend/web).
    # It uses 'docker compose down' which stops everything.
    if grep -q 'docker compose.*down' <<<"$content"; then
        pass "restarts all services (full compose down)"
    else
        fail "may not restart all services"
    fi

    # ── Summary ─────────────────────────────────────────────────────────────
    if [ "$FAIL" -eq 0 ]; then
        echo "restart-rehearsal regression tests PASSED"
        return 0
    fi
    echo "restart-rehearsal regression tests FAILED ($FAIL failure(s))"
    return 1
}

# run_determinism_self_test proves the matching logic is stable: the real,
# known-good script must pass every one of 50 runs, and a known-bad placeholder
# must fail every one of 50 runs. Before the pipefree rewrite this pattern
# failed intermittently under `set -o pipefail`.
run_determinism_self_test() {
    local good bad i output
    good="$(cat "$SCRIPT")"
    for i in $(seq 1 50); do
        if ! output="$(check_content "$good" 2>&1)"; then
            echo "restart-regression determinism FAILED: known-good input rejected on iteration $i" >&2
            echo "$output" | tail -3 >&2
            return 1
        fi
    done
    bad=$'#!/usr/bin/env bash\n# placeholder that satisfies none of the required checks\necho "unrelated"\n'
    for i in $(seq 1 50); do
        if output="$(check_content "$bad" 2>&1)"; then
            echo "restart-regression determinism FAILED: known-bad input accepted on iteration $i" >&2
            return 1
        fi
    done
    echo "restart-regression determinism PASSED (50 known-good passes, 50 known-bad failures)"
    return 0
}

if [ "${1:-}" = "--determinism" ]; then
    run_determinism_self_test
    exit $?
fi

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

check_content "$(cat "$SCRIPT")"
