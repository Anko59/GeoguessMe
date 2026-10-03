#!/usr/bin/env bash
# Real Make contracts and image-retag regressions, with fake Docker only.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
TMP="$(mktemp -d /tmp/geoguessme-local-images.XXXXXX)"
cleanup() {
    local resolved
    resolved="$(realpath "$TMP")"
    case "$resolved" in /tmp/geoguessme-local-images.*) ;; *) return 1 ;; esac
    [ "$resolved" = "$TMP" ] && [ ! -L "$TMP" ] || return 1
    rm -rf -- "$resolved"
}
trap cleanup EXIT
fail() {
    echo "FAIL: $*" >&2
    exit 1
}
export REAL_MAKE="$(command -v make)" IMAGE_TEST_ROOT="$ROOT" IMAGE_TEST_STATE="$TMP/state" IMAGE_TEST_LOG="$TMP/docker.log"
mkdir -p "$TMP/bin" "$IMAGE_TEST_STATE" "$TMP/fixture/tools/quality/test"
cp "$ROOT/tools/quality/test/check-build-caching.sh" "$TMP/fixture/tools/quality/test/"
backend_id="sha256:$(printf 'a%.0s' {1..64})"
web_id="sha256:$(printf 'b%.0s' {1..64})"
keycloak_id="sha256:$(printf 'c%.0s' {1..64})"
export IMAGE_BACKEND_ID="$backend_id" IMAGE_WEB_ID="$web_id" IMAGE_KEYCLOAK_ID="$keycloak_id"

cat >"$TMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker|%s\n' "$*" >>"${IMAGE_TEST_LOG:?}"
args=("$@")
last="${args[${#args[@]} - 1]}"
family_for() {
    case "$1" in
        "${EXPECTED_BACKEND:-}" | "${IMAGE_BACKEND_ID:?}") printf backend ;;
        "${EXPECTED_WEB:-}" | "${IMAGE_WEB_ID:?}") printf web ;;
        geoguessme-backend:cache-test-*) printf backend ;;
        geoguessme-web:cache-test-*) printf web ;;
        geoguessme-keycloak:cache-test-*) printf keycloak ;;
        *) echo "unexpected image reference: $1" >&2; exit 84 ;;
    esac
}
id_for() {
    case "$1" in backend) printf '%s' "$IMAGE_BACKEND_ID" ;; web) printf '%s' "$IMAGE_WEB_ID" ;; keycloak) printf '%s' "$IMAGE_KEYCLOAK_ID" ;; esac
}
if [ "${1:-}" = image ] && [ "${2:-}" = inspect ]; then
    family="$(family_for "$last")"
    format="$*"
    if [ "${IMAGE_TEST_MODE:-}" = scratch ]; then
        [ -s "$IMAGE_TEST_STATE/$family" ] || exit 1
    fi
    case "$format" in
        *'{{.Id}}'*)
            if [ "${IMAGE_TEST_MODE:-}" = consumer ]; then
                [ ! -e "$IMAGE_TEST_STATE/$family.resolved" ] || { echo 'reference resolved twice after retag' >&2; exit 84; }
                : >"$IMAGE_TEST_STATE/$family.resolved"
                printf 'resolve|%s\n' "$last" >>"$IMAGE_TEST_LOG"
                # The original tag now points at an unrelated image. Subsequent
                # metadata/runtime operations must use the captured ID instead.
            fi
            if [ "${IMAGE_TEST_MODE:-}" = scratch ] && [ "${IMAGE_SCRATCH_TAMPER:-}" = 1 ] && [ "$family" = backend ]; then
                count=0
                [ ! -s "$IMAGE_TEST_STATE/id-count" ] || count="$(<"$IMAGE_TEST_STATE/id-count")"
                count=$((count + 1))
                printf '%s' "$count" >"$IMAGE_TEST_STATE/id-count"
                if [ "$count" -ge 3 ]; then printf 'sha256:%064d' 9; exit 0; fi
            fi
            id_for "$family"
            ;;
        *Config.User* | *Config.Healthcheck* | *Architecture*)
            [ "$last" = "$(id_for "$family")" ] || { echo 'mutable reference reused after retag' >&2; exit 84; }
            case "$format" in *Config.User*) printf appuser ;; *Config.Healthcheck*) printf '[CMD healthcheck]' ;; *Architecture*) printf amd64 ;; esac
            ;;
        *) exit 0 ;;
    esac
    exit 0
fi
if [ "${1:-}" = image ] && [ "${2:-}" = rm ]; then
    [ "${IMAGE_TEST_MODE:-}" = scratch ] || { echo 'unexpected image removal' >&2; exit 84; }
    family="$(family_for "$last")"
    [ "$last" = "$(<"$IMAGE_TEST_STATE/$family")" ] || { echo 'removal outside owned scratch tags' >&2; exit 84; }
    printf 'remove|%s\n' "$last" >>"$IMAGE_TEST_LOG"
    : >"$IMAGE_TEST_STATE/$family"
    exit 0
fi
if [ "${1:-}" = build ]; then
    tag=""
    for ((i = 0; i < ${#args[@]}; i++)); do
        if [ "${args[i]}" = -t ]; then tag="${args[i + 1]:-}"; fi
    done
    [[ "$tag" != *@sha256:* ]] || { echo 'attempted to build a promotion digest' >&2; exit 84; }
    printf 'build|%s\n' "$tag" >>"$IMAGE_TEST_LOG"
    exit 0
fi
if [ "${1:-}" = compose ]; then
    [ "${WEB_IMAGE:-}" = "$IMAGE_WEB_ID" ] || { echo 'Compose received mutable web image' >&2; exit 84; }
    if [ "${IMAGE_CONSUMER:-}" != watch ]; then
        [ "${BACKEND_IMAGE:-}" = "$IMAGE_BACKEND_ID" ] || { echo 'Compose received mutable backend image' >&2; exit 84; }
    fi
    if [[ "$*" == *' up '* ]]; then exit 86; fi
    exit 0
fi
if [ "${1:-}" = run ]; then
    if [ "${IMAGE_CONSUMER:-}" = watch ]; then
        [[ " $* " == *" $IMAGE_WEB_ID "* ]] || { echo 'mock gateway used mutable image' >&2; exit 84; }
        exit 0
    fi
    [[ " $* " == *" $IMAGE_BACKEND_ID "* ]] || { echo 'ELF probe used mutable image' >&2; exit 84; }
    printf '62 0'
    exit 0
fi
case "${1:-}" in network | rm) exit 0 ;; *) echo "unexpected Docker operation: $*" >&2; exit 84 ;; esac
DOCKER

cat >"$TMP/bin/make" <<'MAKE'
#!/usr/bin/env bash
set -euo pipefail
if [ "${IMAGE_TEST_MODE:-}" != scratch ]; then exec "${REAL_MAKE:?}" "$@"; fi
for assignment in "$@"; do
    case "$assignment" in LOCAL_BACKEND_IMAGE=* | LOCAL_WEB_IMAGE=* | LOCAL_KEYCLOAK_IMAGE=*) export "$assignment" ;; esac
 done
for pair in "backend|${LOCAL_BACKEND_IMAGE:?}" "web|${LOCAL_WEB_IMAGE:?}" "keycloak|${LOCAL_KEYCLOAK_IMAGE:?}"; do
    family=${pair%%|*}
    ref=${pair#*|}
    case "$ref" in geoguessme-*:cache-test-*) ;; *) echo 'scratch build targeted a normal image' >&2; exit 84 ;; esac
    printf '%s' "$ref" >"${IMAGE_TEST_STATE:?}/$family"
    printf 'scratch-build|%s\n' "$ref" >>"${IMAGE_TEST_LOG:?}"
done
case "$1" in build-images) printf 'CACHED\nUsing cache\n' ;; clean-build) printf 'uncached build\n' ;; *) exit 84 ;; esac
MAKE
chmod +x "$TMP/bin/docker" "$TMP/bin/make"
export PATH="$TMP/bin:$PATH"

consumers=(deployment/scripts/container-verify.sh deployment/scripts/prod-container-verify.sh deployment/scripts/watch/rehearsal.sh)
for helper in "${consumers[@]}"; do
    if grep -Eq 'geoguessme-(backend|web):local' "$ROOT/$helper"; then fail "$helper retains a common image fallback"; fi
    for mode in local signed; do
        : >"$IMAGE_TEST_LOG"
        # Fresh state per subprocess avoids deleting marker paths from a mock.
        state="$TMP/state-$mode-${helper##*/}"
        mkdir -p "$state"
        scope=(env -u BACKEND_IMAGE -u WEB_IMAGE -u LOCAL_BACKEND_IMAGE -u LOCAL_WEB_IMAGE)
        if [ "$mode" = local ]; then
            backend_ref=geoguessme-backend:local-scoped-test
            web_ref=geoguessme-web:local-scoped-test
            scope+=("LOCAL_BACKEND_IMAGE=$backend_ref" "LOCAL_WEB_IMAGE=$web_ref")
        else
            backend_ref="ghcr.io/fixture/backend@sha256:$(printf 'd%.0s' {1..64})"
            web_ref="ghcr.io/fixture/web@sha256:$(printf 'e%.0s' {1..64})"
            scope+=("BACKEND_IMAGE=$backend_ref" "WEB_IMAGE=$web_ref" "LOCAL_BACKEND_IMAGE=wrong:backend" "LOCAL_WEB_IMAGE=wrong:web")
        fi
        consumer=verify
        [ "$helper" != deployment/scripts/watch/rehearsal.sh ] || consumer=watch
        status=0
        "${scope[@]}" GEOGUESSME_TOOLS_PROJECT=geoguessme-tools-image-fixture GEOGUESSME_TEST_PORT_BASE=32100 \
            IMAGE_TEST_MODE=consumer IMAGE_CONSUMER="$consumer" IMAGE_TEST_STATE="$state" \
            EXPECTED_BACKEND="$backend_ref" EXPECTED_WEB="$web_ref" bash "$ROOT/$helper" >"$TMP/output" 2>&1 || status=$?
        expected_status=86
        [ "$helper" != deployment/scripts/container-verify.sh ] || expected_status=0
        if [ "$status" -ne "$expected_status" ]; then
            printf 'consumer output: %s\n' "$(<"$TMP/output")" >&2
            fail "$helper $mode mode returned $status, expected $expected_status"
        fi
        grep -Fxq "resolve|$web_ref" "$IMAGE_TEST_LOG" || fail "$helper did not resolve the selected web image"
        if [ "$consumer" != watch ]; then grep -Fxq "resolve|$backend_ref" "$IMAGE_TEST_LOG" || fail "$helper did not resolve the backend"; fi
    done
    : >"$IMAGE_TEST_LOG"
    if env -u BACKEND_IMAGE -u WEB_IMAGE -u LOCAL_BACKEND_IMAGE -u LOCAL_WEB_IMAGE \
        GEOGUESSME_TOOLS_PROJECT=geoguessme-tools-image-fixture GEOGUESSME_TEST_PORT_BASE=32100 \
        IMAGE_TEST_MODE=consumer bash "$ROOT/$helper" >"$TMP/output" 2>&1; then
        fail "$helper accepted undefined scoped image references"
    fi
    [ ! -s "$IMAGE_TEST_LOG" ] || fail "$helper contacted Docker before rejecting undefined local refs"
    echo "PASS: $helper selects scoped/caller refs and retains immutable IDs after retag"
done

cat >"$TMP/build-probe.mk" <<'PROBE'
include $(IMAGE_TEST_ROOT)/tools/make/setup.mk
include $(IMAGE_TEST_ROOT)/tools/make/deployment.mk
.PHONY: local-refs
local-refs:
	@printf '%s\n' "$${LOCAL_BACKEND_IMAGE}" "$${LOCAL_WEB_IMAGE}" "$${LOCAL_KEYCLOAK_IMAGE}"
PROBE
mkdir -p "$TMP/checkout one" "$TMP/checkout-two"
default_refs() {
    env -u LOCAL_BACKEND_IMAGE -u LOCAL_WEB_IMAGE -u LOCAL_KEYCLOAK_IMAGE \
        -u BACKEND_IMAGE -u WEB_IMAGE -u KEYCLOAK_IMAGE -u GEOGUESSME_TOOLS_PROJECT \
        -u MAKEFLAGS -u MAKEOVERRIDES \
        "$REAL_MAKE" --no-print-directory -s -C "$1" -f "$TMP/build-probe.mk" local-refs
}
first="$(default_refs "$TMP/checkout one")"
[ "$first" = "$(default_refs "$TMP/checkout one")" ] || fail 'local image defaults are not stable'
[ "$first" != "$(default_refs "$TMP/checkout-two")" ] || fail 'different checkouts share local image defaults'
for family in backend web keycloak; do
    grep -Eq "^geoguessme-$family:local-geoguessme-tools-[0-9]+$" <<<"$first" || fail "unscoped default $family image"
done
: >"$IMAGE_TEST_LOG"
# The real Make recipes execute against fake Docker. Signed caller references
# must remain consumer inputs; -t must exclusively receive custom LOCAL tags.
env -u MAKEFLAGS -u MAKEOVERRIDES IMAGE_TEST_MODE=build \
    LOCAL_BACKEND_IMAGE=fixture/backend:custom LOCAL_WEB_IMAGE=fixture/web:custom \
    LOCAL_KEYCLOAK_IMAGE=fixture/keycloak:custom \
    BACKEND_IMAGE="ghcr.io/fixture/backend@sha256:$(printf 'd%.0s' {1..64})" \
    WEB_IMAGE="ghcr.io/fixture/web@sha256:$(printf 'e%.0s' {1..64})" \
    "$REAL_MAKE" --no-print-directory -s -C "$TMP" -f "$TMP/build-probe.mk" build-images clean-build >"$TMP/build-output" 2>&1 || {
    printf '%s\n' "$(<"$TMP/build-output")" >&2
    fail 'real Make build recipes failed with fake Docker'
}
for ref in fixture/backend:custom fixture/web:custom fixture/keycloak:custom; do
    grep -Fxq "build|$ref" "$IMAGE_TEST_LOG" || fail "Make ignored custom local tag $ref"
done
if grep '^build|' "$IMAGE_TEST_LOG" | grep -F '@sha256:' >/dev/null; then fail 'Make tagged a signed promotion digest'; fi

: >"$IMAGE_TEST_LOG"
IMAGE_TEST_MODE=scratch BACKEND_IMAGE=signed:do-not-remove WEB_IMAGE=signed:do-not-remove \
    bash "$TMP/fixture/tools/quality/test/check-build-caching.sh" >"$TMP/cache-output" 2>&1 || {
    printf '%s\n' "$(<"$TMP/cache-output")" >&2
    fail 'scratch caching helper failed its fake build lifecycle'
}
for family in backend web keycloak; do
    grep -Eq "^remove\\|geoguessme-$family:cache-test-" "$IMAGE_TEST_LOG" || fail "scratch $family tag was not cleaned"
done
if grep '^remove|' "$IMAGE_TEST_LOG" | grep -F 'do-not-remove' >/dev/null; then fail 'scratch cleanup touched promotion references'; fi
if grep -F 'docker|image rm -f' "$IMAGE_TEST_LOG" >/dev/null; then fail 'scratch cleanup force-deleted an image'; fi

: >"$IMAGE_TEST_LOG"
if IMAGE_TEST_MODE=scratch IMAGE_SCRATCH_TAMPER=1 \
    bash "$TMP/fixture/tools/quality/test/check-build-caching.sh" >"$TMP/tamper-output" 2>&1; then
    fail 'scratch cleanup accepted a changed image ID'
fi
grep -Fq 'refusing to remove a changed scratch image' "$TMP/tamper-output" || fail 'scratch cleanup failed for the wrong reason'
if grep '^remove|' "$IMAGE_TEST_LOG" >/dev/null; then fail 'scratch cleanup removed a changed image reference'; fi

echo 'local-images regression PASSED: checkout-scoped defaults, caller selection, immutable ID snapshots, custom build tags, safe scratch cleanup'
