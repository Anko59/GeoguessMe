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
case "${1:-}" in
    ps) [ ! -f "$NAMESPACE_STARTED" ] || echo fixture-backend; exit 0 ;;
    image) echo sha256:fixture; exit 0 ;;
    inspect)
        case "$3" in
            *working_dir*) printf '%s\n' "$NAMESPACE_ROOT" ;;
            '{{.Id}} {{.Image}}') echo 'fixture-backend sha256:fixture' ;;
            '{{.Image}}') echo sha256:fixture ;;
            *org.opencontainers.image.revision*) echo '<no value>' ;;
            *) exit 85 ;;
        esac
        exit 0
        ;;
esac
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
if [[ " $* " == *' up '* ]]; then : >"$NAMESPACE_STARTED"; fi
if [[ " $* " == *' ps -q backend '* ]]; then echo fixture-backend; fi
exit 0
DOCKER
printf '#!/bin/sh\nprintf "1111111111111111111111111111111111111111\\n"\n' >"$TMP/bin/git"
chmod +x "$TMP/bin/docker" "$TMP/bin/git"
export PATH="$TMP/bin:$PATH" NAMESPACE_LOG="$TMP/docker.log"

# Loading the real shared Make fragment ensures the test exercises the default
# expression and export, rather than copying its hash algorithm into the test.
cat >"$TMP/probe.mk" <<'MAKE'
include $(NAMESPACE_REPO)/tools/make/setup.mk
.PHONY: namespace-probe app-probe port-probe
namespace-probe:
	@printf '%s\n' "$${GEOGUESSME_TOOLS_PROJECT}"
	@$(COMPOSE_TOOLS) config --quiet
app-probe:
	@printf '%s\n' '$(COMPOSE_DEV)' '$(COMPOSE_PROD)' '$(COMPOSE_IDENTITY)' '$(COMPOSE_TEST)'
port-probe:
	@printf '%s|%s|%s|%s|%s\n' "$${GEOGUESSME_TEST_PORT_BASE}" "$${GEOGUESSME_TEST_WEB_PORT}" "$${GEOGUESSME_TEST_MAILPIT_PORT}" "$${GEOGUESSME_TEST_DB_PORT}" "$${GEOGUESSME_TEST_TOXIPROXY_PORT}"
MAKE
export NAMESPACE_REPO="$REPO"
mkdir -p "$TMP/checkout one" "$TMP/checkout-two"
probe() {
    local -a scope=(env -u GEOGUESSME_TOOLS_PROJECT -u MAKEFLAGS -u MAKEOVERRIDES -u EXPECTED_NAMESPACE
        -u LOCAL_BACKEND_IMAGE -u LOCAL_WEB_IMAGE -u LOCAL_KEYCLOAK_IMAGE -u BACKEND_IMAGE -u WEB_IMAGE -u KEYCLOAK_IMAGE
        -u GEOGUESSME_TEST_PORT_BASE -u GEOGUESSME_TEST_WEB_PORT -u GEOGUESSME_TEST_MAILPIT_PORT
        -u GEOGUESSME_TEST_DB_PORT -u GEOGUESSME_TEST_TOXIPROXY_PORT -u GEOGUESSME_TEST_PUBLIC_URL)
    if [ -n "${2:-}" ]; then scope+=("GEOGUESSME_TOOLS_PROJECT=$2"); fi
    "${scope[@]}" NAMESPACE_PROBE=1 make --no-print-directory -s -C "$1" -f "$TMP/probe.mk" "${3:-namespace-probe}" "${@:4}"
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

ports="$(probe "$TMP/checkout one" '' port-probe)"
[ "$ports" = "$(probe "$TMP/checkout one" '' port-probe)" ] || fail 'same checkout changed default ports'
[ "$ports" != "$(probe "$TMP/checkout-two" '' port-probe)" ] || fail 'different checkout fixtures share default ports'
IFS='|' read -r base web mail db toxi <<<"$ports"
if ((base < 20000 || base > 49990)); then fail 'default port base outside supported range'; fi
if ((web != base || mail != base + 1 || db != base + 2 || toxi != base + 3)); then
    fail 'default port offsets changed'
fi
custom_ports="$(probe "$TMP/checkout one" '' port-probe GEOGUESSME_TEST_PORT_BASE=33000 GEOGUESSME_TEST_WEB_PORT=33100 GEOGUESSME_TEST_MAILPIT_PORT=33101 GEOGUESSME_TEST_DB_PORT=33102 GEOGUESSME_TEST_TOXIPROXY_PORT=33103)"
[ "$custom_ports" = '33000|33100|33101|33102|33103' ] || fail 'caller port overrides were ignored'

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
    export NAMESPACE_HELPER="$helper" NAMESPACE_ROOT="$TMP/fixture"
    export NAMESPACE_STARTED="$TMP/started-$helper-$mode"
    mkdir -p "$(dirname "$NAMESPACE_STARTED")"
    export BACKEND_IMAGE=geoguessme-backend:local-private TOOLS_UID=1000 TOOLS_GID=1000
    export GEOGUESSME_TEST_PORT_BASE=32100 GEOGUESSME_TEST_WEB_PORT=32100 GEOGUESSME_TEST_MAILPIT_PORT=32101
    export GEOGUESSME_TEST_DB_PORT=32102 GEOGUESSME_TEST_TOXIPROXY_PORT=32103
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
    expected=""
    case "$helper" in
        tools/quality/run-e2e.sh) expected=geoguessme-tools-namespace-test-e2e ;;
        tools/quality/run-integration.sh) expected=geoguessme-tools-namespace-test-integration ;;
        deployment/scripts/migration-concurrency.sh) expected=geoguessme-migration-test-geoguessme-tools-namespace-test ;;
        deployment/scripts/restart-rehearsal.sh) expected=geoguessme-restart-rehearsal-geoguessme-tools-namespace-test ;;
        deployment/scripts/load-test.sh) expected=geoguessme-load-geoguessme-tools-namespace-test ;;
        deployment/scripts/reconnect-rehearsal.sh) expected=geoguessme-reconnect-rehearsal-geoguessme-tools-namespace-test ;;
        deployment/scripts/backup-restore-rehearsal.sh) expected=geoguessme-backup-rehearsal-geoguessme-tools-namespace-test ;;
    esac
    if [ -n "$expected" ]; then
        app="$(sed -n 's/^app|//p' "$NAMESPACE_LOG" | sort -u)"
        [[ "$app" =~ ^${expected}-[0-9]+$ ]] ||
            fail "$helper did not retain one checkout/PID-scoped ephemeral project"
    fi
    # This deliberately invalid Compose name is only passed to fake Docker: it
    # proves the command-array/string boundaries do not split an exported value.
    : >"$NAMESPACE_LOG"
    run_helper "$helper" quoted >"$TMP/output" 2>&1 && fail "$helper unexpectedly completed fake tools"
    case "$helper" in
        tools/quality/run-e2e.sh | tools/quality/run-integration.sh)
            [ ! -s "$NAMESPACE_LOG" ] || fail "$helper touched Docker with an invalid project"
            grep -q 'must match' "$TMP/output" || fail "$helper failed for the wrong reason"
            ;;
        *)
            grep -Fxq 'tools|geoguessme-tools-literal space' "$NAMESPACE_LOG" ||
                fail "$helper does not preserve one project argument"
            ;;
    esac
    if grep -Fq 'incorrect tool project' "$TMP/output"; then fail "$helper split its project argument"; fi
    echo "PASS: $helper requires and propagates the tool namespace"
done

echo 'tools-namespace regression PASSED: stable checkout scope, explicit override, strict helpers, unchanged application projects'
