#!/usr/bin/env bash
# Check checkout-scoped tool projects without contacting a Docker daemon.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
TMP="$(mktemp -d /tmp/geoguessme-tools-namespace.XXXXXX)"
cleanup() {
    local resolved
    resolved="$(realpath "$TMP")"
    case "$resolved" in
        /tmp/geoguessme-tools-namespace.*) ;;
        *)
            echo "refusing unexpected fixture cleanup path" >&2
            return 1
            ;;
    esac
    [ "$resolved" = "$TMP" ] && [ ! -L "$TMP" ] || return 1
    rm -rf -- "$resolved"
}
trap cleanup EXIT
fail() {
    echo "FAIL: $*" >&2
    exit 1
}

helpers=(
    tools/quality/run-e2e.sh
    tools/quality/run-integration.sh
    tools/quality/test/check-tool-image-split.sh
    deployment/scripts/migration-concurrency.sh
    deployment/scripts/restart-rehearsal.sh
    deployment/scripts/smoke-test.sh
    deployment/scripts/wait-for-health.sh
    deployment/scripts/load-test.sh
    deployment/scripts/reconnect-rehearsal.sh
    deployment/scripts/backup-restore-rehearsal.sh
)

mkdir -p "$TMP/bin" "$TMP/fixture/frontend" "$TMP/fixture/tools/quality/e2e" \
    "$TMP/fixture/backend/internal/database/migrations"
for helper in "${helpers[@]}"; do
    mkdir -p "$TMP/fixture/$(dirname "$helper")"
    cp "$REPO/$helper" "$TMP/fixture/$helper"
    grep -Fq "\${GEOGUESSME_TOOLS_PROJECT:?Run through Make}" "$REPO/$helper" ||
        fail "$helper does not require the exported project"
    if grep -Eq -- '-p[[:space:]]+geoguessme-tools([[:space:]]|$)' "$REPO/$helper"; then
        fail "$helper retains the global tool project"
    fi
done
cp "$REPO/tools/quality/e2e/arguments.sh" "$TMP/fixture/tools/quality/e2e/arguments.sh"
printf '%s\n' '-- Namespace fixture: never executed against a database.' \
    >"$TMP/fixture/backend/internal/database/migrations/001.sql"

cat >"$TMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
[ "${1:-}" = compose ] || { echo "unexpected Docker command" >&2; exit 85; }
args=("$@")
project=""
tools=false
for ((i = 0; i < ${#args[@]}; i++)); do
    if [ "${args[i]}" = -p ]; then project="${args[i + 1]:-}"; fi
    if [ "${args[i]}" = -f ] && [[ "${args[i + 1]:-}" == *compose.tools.yaml ]]; then
        tools=true
    fi
done
if "$tools"; then
    if [ -n "${EXPECTED_NAMESPACE:-}" ] && [ "$project" != "$EXPECTED_NAMESPACE" ]; then
        printf 'incorrect tool project: <%s>\n' "$project" >&2
        exit 85
    fi
    printf 'tools|%s\n' "$project" >>"${NAMESPACE_LOG:?}"
    if [ "${NAMESPACE_PROBE:-}" = 1 ]; then exit 0; fi
    # Let runners finish their fake readiness helper, then stop at the actual
    # tool invocation. No embedded shell, database, browser or image is executed.
    case "${NAMESPACE_HELPER:-}" in
        tools/quality/run-e2e.sh | tools/quality/run-integration.sh)
            if [[ "$*" == *go-tools*bash* ]]; then printf 'ready\n'; exit 0; fi
            ;;
    esac
    if [ "${NAMESPACE_HELPER:-}" = deployment/scripts/restart-rehearsal.sh ] &&
        [[ "$*" == *go-tools*curl* ]]; then printf '200'; exit 0; fi
    exit 86
fi
printf 'app|%s\n' "$project" >>"${NAMESPACE_LOG:?}"
exit 0
DOCKER
chmod +x "$TMP/bin/docker"
export PATH="$TMP/bin:$PATH" NAMESPACE_LOG="$TMP/docker.log"

# Loading the real shared Make fragment ensures the test exercises the default
# expression and export, rather than copying its hash algorithm into the test.
cat >"$TMP/probe.mk" <<'MAKE'
include $(NAMESPACE_REPO)/tools/make/setup.mk
.PHONY: namespace-probe app-probe
namespace-probe:
	@printf '%s\n' "$${GEOGUESSME_TOOLS_PROJECT}"
	@$(COMPOSE_TOOLS) config --quiet
app-probe:
	@printf '%s\n' '$(COMPOSE_DEV)' '$(COMPOSE_PROD)' '$(COMPOSE_IDENTITY)' '$(COMPOSE_TEST)'
MAKE
export NAMESPACE_REPO="$REPO"
mkdir -p "$TMP/checkout one" "$TMP/checkout-two"
probe() {
    local -a scope=(env -u GEOGUESSME_TOOLS_PROJECT -u MAKEFLAGS -u MAKEOVERRIDES -u EXPECTED_NAMESPACE
        -u LOCAL_BACKEND_IMAGE -u LOCAL_WEB_IMAGE -u LOCAL_KEYCLOAK_IMAGE -u BACKEND_IMAGE -u WEB_IMAGE -u KEYCLOAK_IMAGE)
    if [ -n "${2:-}" ]; then scope+=("GEOGUESSME_TOOLS_PROJECT=$2"); fi
    "${scope[@]}" NAMESPACE_PROBE=1 make --no-print-directory -s -C "$1" -f "$TMP/probe.mk" namespace-probe
}
: >"$NAMESPACE_LOG"
first="$(probe "$TMP/checkout one")"
again="$(probe "$TMP/checkout one")"
second="$(probe "$TMP/checkout-two")"
[[ "$first" =~ ^geoguessme-tools-[0-9]+$ ]] || fail "invalid default project: $first"
[ "$first" = "$again" ] || fail "same checkout changed namespace"
[ "$first" != "$second" ] || fail "different checkouts share a namespace"
grep -Fxq "tools|$first" "$NAMESPACE_LOG" || fail "Compose did not receive the default project"
custom="$(probe "$TMP/checkout one" geoguessme-tools-explicit-123)"
[ "$custom" = geoguessme-tools-explicit-123 ] || fail "explicit namespace override ignored"
grep -Fxq "tools|$custom" "$NAMESPACE_LOG" || fail "Compose did not receive the explicit project"

# Tool-cache isolation must not rename application projects or their volumes.
app_defaults="$(cd "$TMP/checkout one" && make --no-print-directory -s -f "$TMP/probe.mk" app-probe)"
for project in geoguessme-dev geoguessme-prod geoguessme-identity; do
    [[ "$app_defaults" == *"-p $project "* ]] || fail "application project changed: $project"
done
[[ "$app_defaults" == *'docker compose -f deployment/compose.test.yaml --project-directory .'* ]] ||
    fail "test Compose default changed"

run_helper() (
    cd "$TMP/fixture"
    unset GEOGUESSME_TEST_PROJECT GEOGUESSME_MIGRATION_PROJECT GEOGUESSME_RESTART_PROJECT \
        GEOGUESSME_LOAD_PROJECT GEOGUESSME_RECONNECT_PROJECT GEOGUESSME_REHEARSAL_PROJECT
    local helper="$1" mode="$2" shell=bash
    case "$helper" in
        deployment/scripts/smoke-test.sh | deployment/scripts/wait-for-health.sh) shell="sh" ;;
    esac
    export NAMESPACE_HELPER="$helper"
    case "$mode" in
        missing) unset GEOGUESSME_TOOLS_PROJECT ;;
        empty) export GEOGUESSME_TOOLS_PROJECT="" ;;
        normal) export GEOGUESSME_TOOLS_PROJECT=geoguessme-tools-namespace-test ;;
        quoted) export GEOGUESSME_TOOLS_PROJECT='geoguessme-tools-literal space' ;;
        *) fail "unexpected helper mode" ;;
    esac
    export EXPECTED_NAMESPACE="${GEOGUESSME_TOOLS_PROJECT:-}"
    "$shell" "$TMP/fixture/$helper"
)
for helper in "${helpers[@]}"; do
    for mode in missing empty; do
        : >"$NAMESPACE_LOG"
        if run_helper "$helper" "$mode" >"$TMP/output" 2>&1; then
            fail "$helper accepted a $mode project"
        fi
        [ ! -s "$NAMESPACE_LOG" ] || fail "$helper touched Docker before rejecting a $mode project"
        grep -Fq 'Run through Make' "$TMP/output" || fail "$helper failed for the wrong reason"
    done
    : >"$NAMESPACE_LOG"
    run_helper "$helper" normal >"$TMP/output" 2>&1 && fail "$helper unexpectedly completed fake tools"
    grep -Fxq 'tools|geoguessme-tools-namespace-test' "$NAMESPACE_LOG" ||
        fail "$helper did not propagate the selected project"
    if grep -Fq 'incorrect tool project' "$TMP/output"; then fail "$helper split or changed the project"; fi
    case "$helper" in
        tools/quality/run-e2e.sh) app=geoguessme-e2e ;;
        tools/quality/run-integration.sh) app=geoguessme-integration ;;
        deployment/scripts/migration-concurrency.sh) app=geoguessme-migration-test ;;
        deployment/scripts/restart-rehearsal.sh) app=geoguessme-restart-rehearsal ;;
        deployment/scripts/load-test.sh) app=geoguessme-load ;;
        deployment/scripts/reconnect-rehearsal.sh) app=geoguessme-reconnect-rehearsal ;;
        deployment/scripts/backup-restore-rehearsal.sh) app=geoguessme-backup-rehearsal ;;
        *) app="" ;;
    esac
    if [ -n "$app" ]; then
        grep -Fxq "app|$app" "$NAMESPACE_LOG" || fail "$helper changed the application project"
        if grep '^app|' "$NAMESPACE_LOG" | grep -Fvx "app|$app" >/dev/null; then
            fail "$helper touched an unexpected application project"
        fi
    fi
    # This deliberately invalid Compose name is only passed to fake Docker: it
    # proves the command-array/string boundaries do not split an exported value.
    : >"$NAMESPACE_LOG"
    run_helper "$helper" quoted >"$TMP/output" 2>&1 && fail "$helper unexpectedly completed fake tools"
    grep -Fxq 'tools|geoguessme-tools-literal space' "$NAMESPACE_LOG" ||
        fail "$helper does not preserve one project argument"
    if grep -Fq 'incorrect tool project' "$TMP/output"; then fail "$helper split its project argument"; fi
    echo "PASS: $helper requires and propagates the tool namespace"
done

echo 'tools-namespace regression PASSED: stable checkout scope, explicit override, strict helpers, unchanged application projects'
