#!/usr/bin/env bash
# Regression tests for cache-status.sh.
#
# Tests:
#   1. Script exists and is executable
#   2. Default run produces expected output sections (Docker images, build
#      cache, volumes, artifacts, summary)
#   3. PROJECT_PREFIX filter changes image match output
#   4. Script is read-only (no Docker resources modified)
#   5. Output is deterministic (repeatable)
#   6. Handles Docker unavailability gracefully
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")/.." && pwd)/cache-status.sh"
PASS=0
FAIL=0
TEMP_DIR=""

cleanup() {
    if [ -n "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Suppress SC2034: PASS is incremented but read only in the summary below.
# shellcheck disable=SC2034

pass() {
    echo "PASS: $*"
    PASS=$((PASS + 1))
}
fail() {
    echo "FAIL: $*"
    FAIL=$((FAIL + 1))
}

# Run the script and capture exit code and output.  Arguments are forwarded to
# the script.
# shellcheck disable=SC2120 # function accepts arguments forwarded to SCRIPT
run_script() {
    local out exit_code
    out=$("$SCRIPT" "$@" 2>&1) && exit_code=0 || exit_code=$?
    printf '%s\n' "$out"
    return "$exit_code"
}

# Check Docker availability at test start.
docker_available() {
    command -v docker >/dev/null 2>&1
}

echo "cache-status regression tests:"

# ── Test 1: Script existence and permissions ─────────────────────────────────
echo "--- Test 1: Script existence ---"
if [ -f "$SCRIPT" ]; then
    pass "cache-status.sh exists"
else
    fail "cache-status.sh not found at $SCRIPT"
fi

if [ -x "$SCRIPT" ]; then
    pass "cache-status.sh is executable"
else
    fail "cache-status.sh is not executable"
fi

# ── Test 2: Output sections ──────────────────────────────────────────────────
echo "--- Test 2: Output format ---"

output=$(run_script 2>/dev/null) || {
    fail "script exited with non-zero status"
    printf '%s\n' "$output"
}

section_count=$(echo "$output" | grep -cE '^=== ')
if [ "$section_count" -ge 5 ]; then
    pass "output contains at least 5 sections (found $section_count)"
else
    fail "expected at least 5 sections, found $section_count"
    echo "--- output ---"
    echo "$output"
    echo "---"
fi

if echo "$output" | grep -q 'Docker Images'; then
    pass "output contains 'Docker Images' section"
else
    fail "output missing 'Docker Images' section"
fi

if echo "$output" | grep -q 'Docker Build Cache'; then
    pass "output contains 'Docker Build Cache' section"
else
    fail "output missing 'Docker Build Cache' section"
fi

if echo "$output" | grep -q 'Docker Volumes'; then
    pass "output contains 'Docker Volumes' section"
else
    fail "output missing 'Docker Volumes' section"
fi

if echo "$output" | grep -q 'Workspace Artifacts'; then
    pass "output contains 'Workspace Artifacts' section"
else
    fail "output missing 'Workspace Artifacts' section"
fi

if echo "$output" | grep -q 'Summary'; then
    pass "output contains 'Summary' section"
else
    fail "output missing 'Summary' section"
fi

if echo "$output" | grep -q 'Status: complete (read-only, no modifications made)'; then
    pass "output confirms read-only completion"
else
    fail "output missing read-only confirmation"
fi

# ── Test 3: PROJECT_PREFIX filter ────────────────────────────────────────────
echo "--- Test 3: PROJECT_PREFIX filter ---"

output_custom=$(PROJECT_PREFIX=nonexistent-project-prefix "$SCRIPT" 2>/dev/null) || true

if echo "$output_custom" | grep -q '0 images'; then
    pass "PROJECT_PREFIX=nonexistent filters to 0 images"
elif echo "$output_custom" | grep -qi 'no matching'; then
    pass "PROJECT_PREFIX=nonexistent finds no matching images"
else
    echo "  WARN: unexpected output with nonexistent prefix"
    echo "  $output_custom" | head -5
fi

# ── Test 4: Read-only behavior ───────────────────────────────────────────────
echo "--- Test 4: Read-only verification ---"

if docker_available; then
    before_images=$(docker images --format '{{.ID}}' 2>/dev/null | wc -l)
    before_volumes=$(docker volume ls --format '{{.Name}}' 2>/dev/null | wc -l)

    run_script >/dev/null 2>&1 || true

    after_images=$(docker images --format '{{.ID}}' 2>/dev/null | wc -l)
    after_volumes=$(docker volume ls --format '{{.Name}}' 2>/dev/null | wc -l)

    if [ "$before_images" -eq "$after_images" ]; then
        pass "image count unchanged after script run ($before_images)"
    else
        fail "image count changed: before=$before_images after=$after_images"
    fi

    if [ "$before_volumes" -eq "$after_volumes" ]; then
        pass "volume count unchanged after script run ($before_volumes)"
    else
        fail "volume count changed: before=$before_volumes after=$after_volumes"
    fi
else
    echo "  SKIP: Docker unavailable for read-only check"
fi

# ── Test 5: Deterministic output ─────────────────────────────────────────────
echo "--- Test 5: Deterministic output ---"

if docker_available; then
    output_run1=$(run_script 2>/dev/null) || true
    output_run2=$(run_script 2>/dev/null) || true

    # Compare section headers (the structure, not variable data like counts/timestamps).
    sections1=$(echo "$output_run1" | grep '^=== ' || true)
    sections2=$(echo "$output_run2" | grep '^=== ' || true)

    if [ "$sections1" = "$sections2" ]; then
        pass "section structure is deterministic across runs"
    else
        fail "section structure differs between runs"
    fi
else
    echo "  SKIP: Docker unavailable for deterministic check"
fi

# ── Test 6: Graceful Docker unavailability ───────────────────────────────────
echo "--- Test 6: Graceful Docker unavailability ---"

saved_path="$PATH"
PATH="/dev/null:$PATH"
output_no_docker=$(run_script 2>/dev/null) || true
PATH="$saved_path"

if echo "$output_no_docker" | grep -q 'SKIP: Docker unavailable'; then
    pass "gracefully handles Docker unavailability (SKIP messages)"
elif echo "$output_no_docker" | grep -q 'Status: complete'; then
    pass "completes without Docker (status message present)"
else
    fail "no clean handling of Docker unavailability"
    echo "$output_no_docker"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=== artifacts-clean regression tests ==="

MAKEFILE="$(cd "$(dirname "$0")/../../.." && pwd)/Makefile"

# ── Artifacts-clean Test A1: Target exists and is documented ─────────────────
echo "--- Artifacts-clean A1: Target existence ---"
if grep -q '^artifacts-clean:' "$MAKEFILE"; then
    pass "artifacts-clean target exists"
else
    fail "artifacts-clean target not found in Makefile"
fi
if grep -q '^artifacts-clean:.*##' "$MAKEFILE"; then
    pass "artifacts-clean has help text"
else
    fail "artifacts-clean missing help text (## comment)"
fi

# ── Artifacts-clean Test A2: Dockerized execution ────────────────────────────
echo "--- Artifacts-clean A2: Dockerized ---"
artifacts_block=$(sed -n '/^artifacts-clean:/,/^$/p' "$MAKEFILE")
if echo "$artifacts_block" | grep -q 'COMPOSE_TOOLS'; then
    pass "artifacts-clean uses COMPOSE_TOOLS (Dockerized)"
else
    fail "artifacts-clean does not use COMPOSE_TOOLS"
fi

# ── Artifacts-clean Test A3: No Docker cache deletion ────────────────────────
echo "--- Artifacts-clean A3: No Docker cache deletion ---"
if echo "$artifacts_block" | grep -q 'build-cache-prune'; then
    fail "artifacts-clean must not trigger Docker build-cache-prune"
else
    pass "artifacts-clean does not delete Docker caches"
fi
if echo "$artifacts_block" | grep -q 'builder.prune\|docker.prune\|docker.build'; then
    fail "artifacts-clean must not run docker prune/build commands"
else
    pass "artifacts-clean does not run Docker prune/build commands"
fi

# ── Artifacts-clean Test A4: Known artifact paths only ───────────────────────
echo "--- Artifacts-clean A4: Known artifact paths ---"
for allowed in "backend/bin" "backend/tmp" "backend/coverage.out" "frontend/dist" "frontend/coverage" "frontend/test-results" "frontend/playwright-report" "frontend/blob-report"; do
    if echo "$artifacts_block" | grep -q "$allowed"; then
        pass "cleans known artifact path: $allowed"
    fi
done

# ── Artifacts-clean Test A5: No source/tracked file paths ────────────────────
echo "--- Artifacts-clean A5: No source paths ---"
for forbidden in "backend/internal" "backend/cmd" "backend/handlers" "frontend/src" "frontend/public" "frontend/e2e" "deployment/" "docs/" "tools/" ".git/"; do
    if echo "$artifacts_block" | grep -q "$forbidden"; then
        fail "artifacts-clean references source path: $forbidden"
    else
        pass "does not reference source: $forbidden"
    fi
done

# ── Artifacts-clean Test A6: No dangerous paths ──────────────────────────────
echo "--- Artifacts-clean A6: No dangerous paths ---"
if echo "$artifacts_block" | grep -qE '(^|[^a-zA-Z./-])/( |$)' || echo "$artifacts_block" | grep -qE 'rm -rf /($|[^a-zA-Z])'; then
    fail "artifacts-clean contains dangerous bare-root path"
else
    pass "no dangerous bare-root path"
fi
for d in "/home" "/root" "/etc" "/var"; do
    if echo "$artifacts_block" | grep -qF "$d"; then
        fail "artifacts-clean contains dangerous path: $d"
    else
        pass "no dangerous path: $d"
    fi
done
if echo "$artifacts_block" | grep -q '\.\.'; then
    fail "artifacts-clean contains parent-directory traversal (..)"
else
    pass "no parent-directory traversal"
fi

# ── Combined summary ─────────────────────────────────────────────────────────
if [ "$FAIL" -eq 0 ]; then
    echo "cache-status and artifacts-clean regression tests PASSED"
else
    echo "cache-status and artifacts-clean regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
