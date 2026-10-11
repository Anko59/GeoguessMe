#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../../../.." && pwd)
fixture=$(mktemp -d /tmp/geoguessme-rehearsal-isolation.XXXXXX)
cleanup() {
    if [[ "$fixture" == /tmp/geoguessme-rehearsal-isolation.* && -d "$fixture" &&
        ! -L "$fixture" && "$(realpath "$fixture")" == "$fixture" ]]; then
        rm -rf -- "$fixture"
    fi
}
trap cleanup EXIT
fail() {
    echo "FAIL: $*" >&2
    exit 1
}
pass() { echo "PASS: $*"; }

# Test real Make defaults in distinct checkouts, clearing inherited overrides.
mkdir -p "$fixture/one" "$fixture/two"
cat >"$fixture/probe.mk" <<'MAKE'
include $(REPO)/tools/make/setup.mk
probe:
	@printf '%s|%s|%s|%s|%s\n' '$(GEOGUESSME_TEST_PORT_BASE)' '$(GEOGUESSME_TEST_WEB_PORT)' '$(GEOGUESSME_TEST_MAILPIT_PORT)' '$(GEOGUESSME_TEST_DB_PORT)' '$(GEOGUESSME_TEST_TOXIPROXY_PORT)'
MAKE
probe() (
    unset GEOGUESSME_TEST_PORT_BASE GEOGUESSME_TEST_WEB_PORT GEOGUESSME_TEST_MAILPIT_PORT \
        GEOGUESSME_TEST_DB_PORT GEOGUESSME_TEST_TOXIPROXY_PORT
    timeout 15s make --no-print-directory -s -C "$fixture/$1" -f "$fixture/probe.mk" REPO="$repo" probe "${@:2}"
)
one=$(probe one)
[[ "$one" == "$(probe one)" ]] || fail "default checkout port block is unstable"
IFS='|' read -r base web mail db toxi <<<"$one"
expected=$(printf '%s' "$fixture/one" | cksum | awk '{print 20000 + ($1 % 3000) * 10}')
[[ "$base" == "$expected" && "$web" == "$base" && "$mail" == "$((base + 1))" &&
    "$db" == "$((base + 2))" && "$toxi" == "$((base + 3))" && "$base" -ge 20000 && "$base" -le 49990 ]] ||
    fail "default port block differs from the bounded checkout formula"
# CRC blocks are intentionally not a collision-free reservation mechanism.
IFS='|' read -r other_base _ <<<"$(probe two)"
other_expected=$(printf '%s' "$fixture/two" | cksum | awk '{print 20000 + ($1 % 3000) * 10}')
[[ "$other_base" == "$other_expected" ]] || fail "second checkout does not use its own port formula"
[[ "$(probe one GEOGUESSME_TEST_PORT_BASE=32000)" == '32000|32000|32001|32002|32003' ]] ||
    fail "port base override not propagated"
[[ "$(probe one GEOGUESSME_TEST_PORT_BASE=32000 GEOGUESSME_TEST_WEB_PORT=45000 GEOGUESSME_TEST_MAILPIT_PORT=45001 GEOGUESSME_TEST_DB_PORT=45002 GEOGUESSME_TEST_TOXIPROXY_PORT=45003)" == '32000|45000|45001|45002|45003' ]] ||
    fail "individual port overrides ignored"
pass "Make port defaults are stable, bounded, checkout-derived and independently overridable"

scripts=(prod-container-verify.sh restart-rehearsal.sh reconnect-rehearsal.sh
    migration-concurrency.sh backup-restore-rehearsal.sh load-test.sh smoke-rehearsal.sh watch/rehearsal.sh)
config() (
    local script="$1" mode="${2:-default}" assignments
    unset GEOGUESSME_PROD_VERIFY_PROJECT GEOGUESSME_RESTART_PROJECT GEOGUESSME_RECONNECT_PROJECT \
        GEOGUESSME_MIGRATION_PROJECT GEOGUESSME_REHEARSAL_PROJECT GEOGUESSME_LOAD_PROJECT \
        GEOGUESSME_SMOKE_PROJECT GEOGUESSME_WATCH_PROJECT GEOGUESSME_PROD_VERIFY_WEB_PORT \
        GEOGUESSME_PROD_VERIFY_SMTP_PORT GEOGUESSME_RESTART_WEB_PORT GEOGUESSME_RESTART_DB_PORT \
        GEOGUESSME_RESTART_MAILPIT_PORT GEOGUESSME_RECONNECT_WEB_PORT GEOGUESSME_RECONNECT_MAILPIT_PORT \
        GEOGUESSME_MIGRATION_DB_PORT GEOGUESSME_REHEARSAL_DB_PORT GEOGUESSME_SMOKE_WEB_PORT \
        GEOGUESSME_SMOKE_MAILPIT_PORT GEOGUESSME_WATCH_PRODUCTION_MOCK_PORT GEOGUESSME_WATCH_DEV_MOCK_PORT \
        GEOGUESSME_WATCH_PORT GEOGUESSME_WATCH_DOCKER_PROXY_PORT
    export GEOGUESSME_TOOLS_PROJECT=geoguessme-tools-isolation GEOGUESSME_TEST_PORT_BASE=32000
    export GEOGUESSME_TEST_WEB_PORT=32000 GEOGUESSME_TEST_MAILPIT_PORT=32001
    export GEOGUESSME_TEST_DB_PORT=32002 GEOGUESSME_TEST_TOXIPROXY_PORT=32003
    if [[ "$mode" == override ]]; then
        export GEOGUESSME_PROD_VERIFY_PROJECT=explicit-fixture-rehearsal GEOGUESSME_RESTART_PROJECT=explicit-fixture-rehearsal
        export GEOGUESSME_RECONNECT_PROJECT=explicit-fixture-rehearsal GEOGUESSME_MIGRATION_PROJECT=explicit-fixture-rehearsal
        export GEOGUESSME_REHEARSAL_PROJECT=explicit-fixture-rehearsal GEOGUESSME_LOAD_PROJECT=explicit-fixture-rehearsal
        export GEOGUESSME_SMOKE_PROJECT=explicit-fixture-rehearsal GEOGUESSME_WATCH_PROJECT=explicit-fixture-rehearsal
        export GEOGUESSME_PROD_VERIFY_WEB_PORT=45004 GEOGUESSME_PROD_VERIFY_SMTP_PORT=45005
        export GEOGUESSME_RESTART_WEB_PORT=45000 GEOGUESSME_RESTART_DB_PORT=45002 GEOGUESSME_RESTART_MAILPIT_PORT=45001
        export GEOGUESSME_RECONNECT_WEB_PORT=45000 GEOGUESSME_RECONNECT_MAILPIT_PORT=45001
        export GEOGUESSME_MIGRATION_DB_PORT=45002 GEOGUESSME_REHEARSAL_DB_PORT=45002
        export GEOGUESSME_SMOKE_WEB_PORT=45000 GEOGUESSME_SMOKE_MAILPIT_PORT=45001
        export GEOGUESSME_TEST_WEB_PORT=45000 GEOGUESSME_TEST_MAILPIT_PORT=45001
        export GEOGUESSME_WATCH_PRODUCTION_MOCK_PORT=45006 GEOGUESSME_WATCH_DEV_MOCK_PORT=45007
        export GEOGUESSME_WATCH_PORT=45008 GEOGUESSME_WATCH_DOCKER_PROXY_PORT=45009
    fi
    # Only execute actual scalar configuration assignments, not image lookups,
    # Compose startup, cleanup or any other external fixture operation.
    assignments=$(awk '/^(PROJECT|WEB_PORT|DB_PORT|MAILPIT_PORT|SMTP_WEB_PORT|PRODUCTION_MOCK_PORT|DEV_MOCK_PORT|GEOGUESSME_WATCH_PORT|GEOGUESSME_WATCH_DOCKER_PROXY_PORT)=/' "$repo/deployment/scripts/$script")
    assignments+=$'\nprintf "%s|%s|%s|%s|%s|%s|%s|%s|%s\\n" "$PROJECT" "${WEB_PORT:-}" "${MAILPIT_PORT:-}" "${DB_PORT:-}" "${SMTP_WEB_PORT:-}" "${PRODUCTION_MOCK_PORT:-}" "${DEV_MOCK_PORT:-}" "${GEOGUESSME_WATCH_PORT:-}" "${GEOGUESSME_WATCH_DOCKER_PROXY_PORT:-}"'
    bash -ec "$assignments"
)
for script in "${scripts[@]}"; do
    first=$(config "$script")
    second=$(config "$script")
    IFS='|' read -r project web mail db smtp prod_mock dev_mock watch proxy <<<"$first"
    IFS='|' read -r next_project _ <<<"$second"
    [[ "$project" == *'-geoguessme-tools-isolation-'* && "$project" =~ -[0-9]+$ && "$project" != "$next_project" ]] ||
        fail "$script does not allocate a checkout/PID-scoped project for each invocation"
    case "$script" in
        prod-container-verify.sh)
            [[ "$web|$smtp" == '32004|32005' ]] || fail "production fixture ports are not block offsets4/5"
            ;;
        restart-rehearsal.sh)
            [[ "$web|$mail|$db" == '32000|32001|32002' ]] || fail "restart fixture ports ignore Make defaults"
            ;;
        reconnect-rehearsal.sh | load-test.sh | smoke-rehearsal.sh)
            [[ "$web|$mail" == '32000|32001' ]] || fail "$script web/mail ports ignore Make defaults"
            ;;
        migration-concurrency.sh | backup-restore-rehearsal.sh)
            [[ "$db" == 32002 ]] || fail "$script DB port ignores Make default"
            ;;
        watch/rehearsal.sh)
            [[ "$prod_mock|$dev_mock|$watch|$proxy" == '32006|32007|32008|32009' ]] || fail "watch ports are not block offsets6-9"
            ;;
    esac
    override=$(config "$script" override)
    IFS='|' read -r project web mail db smtp prod_mock dev_mock watch proxy <<<"$override"
    [[ "$project" == explicit-fixture-rehearsal ]] || fail "$script ignores explicit project override"
    case "$script" in
        prod-container-verify.sh) [[ "$web|$smtp" == '45004|45005' ]] || fail "prod port overrides ignored" ;;
        restart-rehearsal.sh) [[ "$web|$mail|$db" == '45000|45001|45002' ]] || fail "restart port overrides ignored" ;;
        reconnect-rehearsal.sh | load-test.sh | smoke-rehearsal.sh) [[ "$web|$mail" == '45000|45001' ]] || fail "$script port overrides ignored" ;;
        migration-concurrency.sh | backup-restore-rehearsal.sh) [[ "$db" == 45002 ]] || fail "$script DB override ignored" ;;
        watch/rehearsal.sh) [[ "$prod_mock|$dev_mock|$watch|$proxy" == '45006|45007|45008|45009' ]] || fail "watch port overrides ignored" ;;
    esac
    pass "$script scopes projects per invocation, derives ports and respects explicit overrides"
done
if grep -Eq '127\.0\.0\.1:(18081|18084)|export GEOGUESSME_WATCH_PORT=18084|export GEOGUESSME_WATCH_DOCKER_PROXY_PORT=12375' "$repo/deployment/scripts/watch/rehearsal.sh"; then
    fail "watch probes still target legacy fixed host ports"
fi
pass "watch publications and probes no longer use shared legacy host ports"
echo 'rehearsal-isolation regression PASSED'
