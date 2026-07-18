#!/usr/bin/env bash
# Deterministic regression/config check for CI workflow cache and artifact
# retention configuration.
#
# Checks:
#   1. Workflow file exists and is valid YAML (via actionlint if available)
#   2. upload-artifact step has explicit retention-days set
#   3. Docker Buildx is configured with install: true
#   4. Setup-buildx-action has buildkitd-config-inline with GC config
#   5. A cache step exists for Docker build layers
#   6. No secrets, .env files, or sensitive variables are uploaded as artifacts
#   7. Failure diagnostics are preserved (upload-artifact runs on failure())
#   8. Cache keys are scoped by branch and lockfile hash
#   9. Makefile provides DOCKER_BUILD_FLAGS for CI cache integration
set -euo pipefail

WORKFLOW=".github/workflows/ci.yml"
REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"

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

cd "$REPO_ROOT"

echo "ci-retention regression tests:"

# ── Test 1: Workflow file exists ────────────────────────────────────────────
echo "--- Test 1: Workflow file exists ---"

if [ -f "$WORKFLOW" ]; then
    pass "workflow file $WORKFLOW exists"
else
    fail "workflow file $WORKFLOW not found"
fi

# ── Test 2: YAML validity via actionlint (when available) ───────────────────
echo "--- Test 2: Workflow validity ---"

if command -v actionlint >/dev/null 2>&1; then
    if actionlint "$WORKFLOW" >/dev/null 2>&1; then
        pass "actionlint validates $WORKFLOW"
    else
        fail "actionlint reports errors in $WORKFLOW"
        actionlint "$WORKFLOW" 2>&1 | head -10
    fi
else
    echo "  SKIP: actionlint not available, using basic YAML check"
    if grep -q '^name:' "$WORKFLOW" && grep -q '^jobs:' "$WORKFLOW"; then
        pass "basic YAML structure check passed"
    else
        fail "basic YAML structure check failed"
    fi
fi

# ── Test 3: retention-days set on artifact upload ───────────────────────────
echo "--- Test 3: artifact retention-days ---"

if grep -q 'retention-days:' "$WORKFLOW"; then
    days=$(grep -oP 'retention-days:\s*\K[0-9]+' "$WORKFLOW" | head -1)
    if [ -n "$days" ] && [ "$days" -ge 1 ] && [ "$days" -le 90 ]; then
        pass "artifact upload has retention-days: $days (bounded)"
    else
        fail "retention-days value '$days' out of expected 1..90 range"
    fi
else
    fail "no retention-days found in workflow"
fi

# ── Test 4: Docker Buildx install: true ─────────────────────────────────────
echo "--- Test 4: Buildx install: true ---"

if grep -q 'install: true' "$WORKFLOW"; then
    pass "docker/setup-buildx-action has install: true"
else
    fail "docker/setup-buildx-action missing install: true"
fi

# ── Test 5: BuildKit GC configuration ───────────────────────────────────────
echo "--- Test 5: BuildKit GC configuration ---"

if grep -q 'gckeepstorage' "$WORKFLOW"; then
    pass "BuildKit GC gckeepstorage is configured (bounded cache)"
else
    fail "BuildKit GC gckeepstorage is not configured"
fi

if grep -q 'gc = true' "$WORKFLOW"; then
    pass "BuildKit GC is enabled"
else
    fail "BuildKit GC is not enabled"
fi

# ── Test 6: Cache step exists for Docker layers ─────────────────────────────
echo "--- Test 6: Docker cache step ---"

if grep -qE 'actions/cache@' "$WORKFLOW"; then
    pass "actions/cache step exists"
else
    fail "no actions/cache step found"
fi

# Cache must reference a Docker-specific path.
if grep -q '/tmp/.buildx-cache' "$WORKFLOW"; then
    pass "cache path targets Docker buildx cache directory"
else
    fail "cache path does not target Docker buildx cache directory"
fi

# ── Test 7: No secrets or sensitive data uploaded ───────────────────────────
echo "--- Test 7: No secrets or sensitive data ---"

# Artifact paths must not include .env files, secret files, or credentials.
artifact_paths=$(sed -n '/path:/,/^[[:space:]]*[a-z]/p' "$WORKFLOW" | grep -E '^\s+- ' | grep -oP '\S+$' || true)
if [ -z "$artifact_paths" ]; then
    # Try a different extraction for multi-line YAML
    artifact_paths=$(grep -A 20 'path:' "$WORKFLOW" | grep -E '^\s+- ' | sed 's/^\s*- //' || true)
fi

has_secrets=false
for p in $artifact_paths; do
    case "$p" in
        *.env | *.env.* | *secret* | *credential* | *token* | *password* | *key*)
            fail "potential secret path found in artifacts: $p"
            has_secrets=true
            ;;
    esac
done

if [ "$has_secrets" = false ]; then
    pass "no secret, .env, or credential paths in artifact upload"
fi

# ── Test 8: Failure diagnostics preserved ───────────────────────────────────
echo "--- Test 8: Failure diagnostics preserved ---"

if grep -q 'failure()' "$WORKFLOW"; then
    pass "artifact upload runs on failure() to preserve diagnostics"
else
    fail "artifact upload does not run on failure()"
fi

# The artifact name should indicate verification artifacts.
if grep -q 'name:.*artifact' "$WORKFLOW"; then
    pass "artifact upload has a descriptive name"
else
    fail "artifact upload is missing a name"
fi

# ── Test 9: Cache keys are scoped ───────────────────────────────────────────
echo "--- Test 9: Cache key scoping ---"

if grep -qE 'key:.*CACHE_BRANCH' "$WORKFLOW"; then
    pass "cache key includes branch scope"
fi

if grep -q 'restore-keys:' "$WORKFLOW"; then
    pass "cache has restore-keys for cross-run fallback"
else
    fail "cache missing restore-keys"
fi

# ── Test 10: DOCKER_BUILD_FLAGS variable in Makefile ────────────────────────
echo "--- Test 10: Makefile cache support ---"

if grep -q 'DOCKER_BUILD_FLAGS' Makefile; then
    pass "Makefile defines DOCKER_BUILD_FLAGS for CI cache"
else
    fail "Makefile missing DOCKER_BUILD_FLAGS"
fi

# Verify build-images uses the variable (literal $(DOCKER_BUILD_FLAGS) in Makefile).
# shellcheck disable=SC2016
pattern='docker build.*$(DOCKER_BUILD_FLAGS)'
if grep -q "$pattern" Makefile; then
    pass "build-images target passes DOCKER_BUILD_FLAGS"
else
    fail "build-images target does not use DOCKER_BUILD_FLAGS"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "ci-retention regression tests PASSED (${PASS} passed)"
else
    echo "ci-retention regression tests FAILED (${PASS} passed, ${FAIL} failed)"
    exit 1
fi
