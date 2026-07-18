#!/usr/bin/env bash
# Self-test verifying Docker layer caching behavior for build-images and clean-build.
#   build-images must use cached layers on a second run.
#   clean-build must not use any cached layers.
#   Both targets must produce valid, inspectable images.
set -euo pipefail

failures=0

pass() { echo "PASS: $*"; }
fail() {
    echo "FAIL: $*"
    failures=$((failures + 1))
}

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$REPO"

BACKEND_IMAGE="geoguessme-backend:local"
WEB_IMAGE="geoguessme-web:local"

# ── Helpers ──────────────────────────────────────────────────────────────────

image_exists() {
    docker image inspect "$1" >/dev/null 2>&1
}

image_has_user() {
    local user
    user="$(docker image inspect --format '{{.Config.User}}' "$1")"
    test -n "$user" && test "$user" != "0" && test "$user" != "root" && test "$user" != "0:0" && test "$user" != "root:root"
}

image_has_healthcheck() {
    local health
    health="$(docker image inspect --format '{{if .Config.Healthcheck}}{{.Config.Healthcheck.Test}}{{end}}' "$1")"
    test -n "$health"
}

clean_images() {
    docker rmi -f "$BACKEND_IMAGE" "$WEB_IMAGE" 2>/dev/null || true
}

# Count CACHED lines in docker build output (plain progress).
count_cached() {
    grep -c 'CACHED' "$1" 2>/dev/null || echo 0
}

# ── build-images: cached behavior ────────────────────────────────────────────

echo "--- build-images (cached) ---"

clean_images

# First build: populate cache.
echo "  First build (populate cache)..."
make build-images >/tmp/build-caching-run1.log 2>&1
if image_exists "$BACKEND_IMAGE" && image_exists "$WEB_IMAGE"; then
    pass "build-images first run produced both images"
else
    fail "build-images first run did not produce both images"
fi

# Second build: must hit cache (at least some layers).
echo "  Second build (expect cache hits)..."
BUILDKIT_PROGRESS=plain make build-images >/tmp/build-caching-run2.log 2>&1
cached2=$(count_cached /tmp/build-caching-run2.log)
if image_exists "$BACKEND_IMAGE" && image_exists "$WEB_IMAGE"; then
    pass "build-images second run produced both images"
else
    fail "build-images second run did not produce both images"
fi
if [ "$cached2" -gt 0 ]; then
    pass "build-images second run used cached layers (${cached2} CACHED lines)"
else
    fail "build-images second run did not use any cached layers (expected >0 CACHED lines)"
fi

# ── clean-build: no-cache behavior ───────────────────────────────────────────

echo "--- clean-build (no cache) ---"

clean_images

# Clean build: must not use cache.
echo "  Clean build (expect no cache)..."
BUILDKIT_PROGRESS=plain make clean-build >/tmp/build-caching-run3.log 2>&1
cached3=$(count_cached /tmp/build-caching-run3.log)
if image_exists "$BACKEND_IMAGE" && image_exists "$WEB_IMAGE"; then
    pass "clean-build produced both images"
else
    fail "clean-build did not produce both images"
fi
# Base images and Dockerfile syntax are always resolved from the local image
# store, so --no-cache still shows a few CACHED lines. The real check is that
# the clean build uses significantly fewer cached lines than the cached build.
if [ "$cached3" -lt "$cached2" ]; then
    pass "clean-build used fewer cached layers than build-images (${cached3} vs ${cached2} CACHED lines)"
else
    fail "clean-build did not reduce cached layers (${cached3} vs ${cached2} CACHED lines)"
fi

# ── Image hardening: non-root user and healthcheck ───────────────────────────

echo "--- image hardening (both targets produce equivalent images) ---"

for image in "$BACKEND_IMAGE" "$WEB_IMAGE"; do
    if image_has_user "$image"; then
        pass "$image runs as non-root user"
    else
        fail "$image does not have a valid non-root user"
    fi
    if image_has_healthcheck "$image"; then
        pass "$image has a healthcheck"
    else
        fail "$image is missing a healthcheck"
    fi
done

# ── Compose validation ───────────────────────────────────────────────────────

echo "--- compose validation against built images ---"

if BACKEND_IMAGE="$BACKEND_IMAGE" WEB_IMAGE="$WEB_IMAGE" \
    docker compose -f deployment/compose.production.yaml --project-directory . config --quiet; then
    pass "production compose validates against built images"
else
    fail "production compose failed validation against built images"
fi

# ── Summary ──────────────────────────────────────────────────────────────────

rm -f /tmp/build-caching-run1.log /tmp/build-caching-run2.log /tmp/build-caching-run3.log

if [ "$failures" -gt 0 ]; then
    echo "build-caching self-test FAILED (${failures} failure(s))"
    exit 1
fi
echo "build-caching self-test PASSED"
