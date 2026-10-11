#!/usr/bin/env bash
# Verify build-images caching and clean-build using disposable image tags only.
set -euo pipefail

failures=0
pass() { echo "PASS: $*"; }
fail() {
    echo "FAIL: $*"
    failures=$((failures + 1))
}
REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$REPO"

SCRATCH_DIR="$(mktemp -d /tmp/geoguessme-build-caching.XXXXXX)"
run_suffix="${SCRATCH_DIR##*.}-$$"
scratch_backend="geoguessme-backend:cache-test-$run_suffix"
scratch_web="geoguessme-web:cache-test-$run_suffix"
scratch_keycloak="geoguessme-keycloak:cache-test-$run_suffix"
readonly SCRATCH_DIR run_suffix scratch_backend scratch_web scratch_keycloak
export LOCAL_BACKEND_IMAGE="$scratch_backend" LOCAL_WEB_IMAGE="$scratch_web" LOCAL_KEYCLOAK_IMAGE="$scratch_keycloak"
declare -A owned_ids=()

clean_images() {
    local image expected current
    for image in "$scratch_backend" "$scratch_web" "$scratch_keycloak"; do
        expected="${owned_ids[$image]:-}"
        [ -n "$expected" ] || continue
        current="$(docker image inspect --format '{{.Id}}' "$image")" || return 1
        if [ "$current" != "$expected" ]; then
            echo "refusing to remove a changed scratch image: $image" >&2
            return 1
        fi
        # Remove only this scratch tag, never the image ID or other aliases.
        # Without --force Docker also refuses removal of an in-use last tag.
        docker image rm "$image" || return 1
        unset 'owned_ids[$image]'
    done
}
cleanup() {
    local status=$? resolved
    if ! clean_images; then
        echo 'scratch image cleanup failed; no shared image references were removed' >&2
        [ "$status" -ne 0 ] || status=1
    fi
    resolved="$(realpath "$SCRATCH_DIR")"
    case "$resolved" in
        /tmp/geoguessme-build-caching.*) ;;
        *)
            echo 'refusing unexpected scratch directory cleanup' >&2
            return 1
            ;;
    esac
    [ "$resolved" = "$SCRATCH_DIR" ] && [ ! -L "$SCRATCH_DIR" ] || return 1
    rm -rf -- "$resolved"
    exit "$status"
}
trap cleanup EXIT

for image in "$scratch_backend" "$scratch_web" "$scratch_keycloak"; do
    if docker image inspect "$image" >/dev/null 2>&1; then
        echo "refusing to reuse an existing scratch image reference: $image" >&2
        exit 1
    fi
done

record_images() {
    local image id
    for image in "$scratch_backend" "$scratch_web" "$scratch_keycloak"; do
        id="$(docker image inspect --format '{{.Id}}' "$image")" || return 1
        [[ "$id" =~ ^sha256:[a-f0-9]{64}$ ]] || return 1
        owned_ids["$image"]="$id"
    done
}
run_build() {
    # Explicit command-line assignments override inherited Make overrides too.
    # Caller promotion references and the normal gate's local tags stay intact.
    BUILDKIT_PROGRESS=plain make "$1" \
        "LOCAL_BACKEND_IMAGE=$scratch_backend" "LOCAL_WEB_IMAGE=$scratch_web" \
        "LOCAL_KEYCLOAK_IMAGE=$scratch_keycloak"
}
image_has_user() {
    local user
    user="$(docker image inspect --format '{{.Config.User}}' "$1")"
    test -n "$user" && test "$user" != 0 && test "$user" != root && test "$user" != 0:0 && test "$user" != root:root
}
image_has_healthcheck() {
    local health
    health="$(docker image inspect --format '{{if .Config.Healthcheck}}{{.Config.Healthcheck.Test}}{{end}}' "$1")"
    test -n "$health"
}
count_cached() {
    local count status=0
    count="$(grep -Ec 'CACHED|Using cache' "$1")" || status=$?
    [ "$status" -le 1 ] || return "$status"
    printf '%s\n' "$count"
}

echo '--- build-images (cached) ---'
echo '  First build (populate cache)...'
run_build build-images >"$SCRATCH_DIR/run1.log" 2>&1
record_images
pass 'build-images first run produced scratch images'
echo '  Second build (expect cache hits)...'
run_build build-images >"$SCRATCH_DIR/run2.log" 2>&1
record_images
cached2="$(count_cached "$SCRATCH_DIR/run2.log")"
pass 'build-images second run produced scratch images'
if [ "$cached2" -gt 0 ]; then
    pass "build-images second run used cached layers ($cached2 cache hits)"
else
    fail 'build-images second run did not use cached layers'
fi

echo '--- clean-build (no cache) ---'
clean_images
echo '  Clean build (expect no cache)...'
run_build clean-build >"$SCRATCH_DIR/run3.log" 2>&1
record_images
cached3="$(count_cached "$SCRATCH_DIR/run3.log")"
pass 'clean-build produced scratch images'
# BuildKit may cache syntax/base resolution even with --no-cache. Legacy Docker
# prints "Using cache" instead of "CACHED"; both formats are counted above.
if [ "$cached3" -lt "$cached2" ]; then
    pass "clean-build used fewer cached layers ($cached3 vs $cached2)"
else
    fail "clean-build did not reduce cached layers ($cached3 vs $cached2)"
fi

echo '--- image hardening ---'
for image in "$scratch_backend" "$scratch_web"; do
    id="${owned_ids[$image]}"
    if image_has_user "$id"; then pass "$image runs as non-root"; else fail "$image has an invalid user"; fi
    if image_has_healthcheck "$id"; then pass "$image has a healthcheck"; else fail "$image has no healthcheck"; fi
done

echo '--- compose validation against immutable scratch artifacts ---'
if BACKEND_IMAGE="${owned_ids[$scratch_backend]}" WEB_IMAGE="${owned_ids[$scratch_web]}" \
    docker compose -f deployment/compose.production.yaml --project-directory . config --quiet; then
    pass 'production compose validates against built scratch images'
else
    fail 'production compose failed validation against built scratch images'
fi
if [ "$failures" -gt 0 ]; then
    echo "build-caching self-test FAILED ($failures failures)"
    exit 1
fi
echo 'build-caching self-test PASSED'
