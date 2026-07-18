#!/usr/bin/env bash
set -euo pipefail

PASS=0
FAIL=0

# ── Stale-artifact regression ────────────────────────────────────────────────
# The E2E runner must clear test-results and playwright-report before each
# invocation so no stale output from a prior run is retained.

stale_dir=$(mktemp -d)
mkdir -p "$stale_dir/frontend/test-results/old-trace" "$stale_dir/frontend/playwright-report/old-report"
echo "stale" >"$stale_dir/frontend/test-results/old-trace/trace.zip"
echo "stale" >"$stale_dir/frontend/playwright-report/old-report/index.html"

# Simulate the clearing logic that run-e2e.sh executes before each run.
rm -rf "$stale_dir/frontend/test-results" "$stale_dir/frontend/playwright-report"
mkdir -p "$stale_dir/frontend/test-results" "$stale_dir/frontend/playwright-report"

if [ -f "$stale_dir/frontend/test-results/old-trace/trace.zip" ] ||
    [ -f "$stale_dir/frontend/playwright-report/old-report/index.html" ]; then
    echo "FAIL: stale-artifact - old artifacts were not cleared"
    FAIL=$((FAIL + 1))
else
    echo "PASS: stale-artifact"
    PASS=$((PASS + 1))
fi
rm -rf "$stale_dir"

# ── Metacharacter-path regression ────────────────────────────────────────────
# The E2E runner must pass GEOGUESSME_E2E_SPEC arguments safely without
# interpolating them into a sh -c string.  Verify that the script uses
# "${test_args[@]}" (safe) and never "${test_args[*]}" (unsafe) in a
# docker compose invocation context.

SCRIPT="$(cd "$(dirname "$0")/.." && pwd)/run-e2e.sh"

meta_fail=0
if ! grep -q 'run --rm --no-deps' "$SCRIPT"; then
    echo "FAIL: metacharacter-path - missing --rm run pattern"
    FAIL=$((FAIL + 1))
    meta_fail=1
fi
if grep -q 'sh -c.*test_args' "$SCRIPT"; then
    echo "FAIL: metacharacter-path - unsafe sh -c interpolation of test_args detected"
    FAIL=$((FAIL + 1))
    meta_fail=1
fi
# shellcheck disable=SC2016 # Literal grep pattern, not a variable reference.
if ! grep -q '"${test_args\[@\]}"' "$SCRIPT"; then
    echo "FAIL: metacharacter-path - safe \"\${test_args[@]}\" expansion not found"
    FAIL=$((FAIL + 1))
    meta_fail=1
fi
if [ "$meta_fail" -eq 0 ]; then
    echo "PASS: metacharacter-path"
    PASS=$((PASS + 1))
fi

# ── Browser-volume regression ────────────────────────────────────────────────
# The pinned Playwright image ships browser binaries; a redundant cache
# volume must not be declared.

COMPOSE_TOOLS="$(cd "$(dirname "$0")/../../.." && pwd)/deployment/compose.tools.yaml"

if grep -q 'playwright-cache' "$COMPOSE_TOOLS"; then
    echo "FAIL: browser-volume - redundant playwright-cache volume still declared"
    FAIL=$((FAIL + 1))
else
    echo "PASS: browser-volume"
    PASS=$((PASS + 1))
fi

echo "e2e-regression results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
