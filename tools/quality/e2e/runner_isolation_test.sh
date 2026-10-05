#!/usr/bin/env bash
# Exercise real runners against fake Docker; no shared stack is touched.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
TMP="$(mktemp -d /tmp/geoguessme-runner-isolation.XXXXXX)"
cleanup() {
    local resolved
    resolved="$(realpath "$TMP")"
    case "$resolved" in /tmp/geoguessme-runner-isolation.*) ;; *) return 1 ;; esac
    [ "$resolved" = "$TMP" ] && [ ! -L "$TMP" ] || return 1
    rm -rf -- "$resolved"
}
trap cleanup EXIT
fail() {
    echo "FAIL: $*" >&2
    exit 1
}
mkdir -p "$TMP/bin" "$TMP/fixture/tools/quality/e2e" "$TMP/fixture/deployment/scripts" "$TMP/fixture/frontend"
cp "$ROOT/tools/quality/run-e2e.sh" "$ROOT/tools/quality/run-integration.sh" "$TMP/fixture/tools/quality/"
cp "$ROOT/tools/quality/e2e/arguments.sh" "$TMP/fixture/tools/quality/e2e/"
printf '#!/usr/bin/env bash\nexit 0\n' >"$TMP/fixture/deployment/scripts/wait-for-health.sh"
chmod +x "$TMP/fixture/deployment/scripts/wait-for-health.sh"
cat >"$TMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
    ps) [ ! -f "$RUNNER_TRACE.started" ] || echo fixture-backend; exit 0 ;;
    image) echo sha256:fixture; exit 0 ;;
    inspect)
        case "$3" in
            *working_dir*) printf '%s\n' "$RUNNER_ROOT" ;;
            '{{.Id}} {{.Image}}') echo 'fixture-backend sha256:fixture' ;;
            '{{.Image}}') echo sha256:fixture ;;
            *org.opencontainers.image.revision*) echo '<no value>' ;;
            *) exit 90 ;;
        esac
        exit 0
        ;;
esac
args=("$@")
project=""
for ((i = 0; i < ${#args[@]}; i++)); do
    [ "${args[i]}" != -p ] || project="${args[i + 1]:-}"
done
if [[ "$*" == *compose.test.yaml* ]]; then
    if [[ " $* " == *' up '* ]]; then : >"$RUNNER_TRACE.started"; fi
    if [[ " $* " == *' ps -q backend '* ]]; then echo fixture-backend; fi
    printf 'app|%s|%s|%s|%s|%s\n' "$project" "${GEOGUESSME_TEST_WEB_PORT:-}" \
        "${GEOGUESSME_TEST_MAILPIT_PORT:-}" "${GEOGUESSME_TEST_DB_PORT:-}" \
        "${GEOGUESSME_TEST_TOXIPROXY_PORT:-}" >>"${RUNNER_TRACE:?}"
    printf 'operation|%s|%s\n' "$project" "$*" >>"$RUNNER_TRACE"
    exit 0
fi
printf 'tools|%s\n' "$project" >>"${RUNNER_TRACE:?}"
exit 23
DOCKER
printf '#!/bin/sh\nprintf "1111111111111111111111111111111111111111\\n"\n' >"$TMP/bin/git"
chmod +x "$TMP/bin/docker" "$TMP/bin/git"
run_case() {
    local helper="$1" namespace="$2" override="$3" trace="$4" status=0
    env -u GEOGUESSME_TEST_PROJECT -u GEOGUESSME_TEST_PUBLIC_URL \
        PATH="$TMP/bin:$PATH" RUNNER_TRACE="$trace" RUNNER_ROOT="$TMP/fixture" \
        BACKEND_IMAGE=geoguessme-backend:local-private GEOGUESSME_TOOLS_PROJECT="$namespace" \
        GEOGUESSME_TEST_PROJECT="$override" GEOGUESSME_TEST_WEB_PORT=32100 \
        GEOGUESSME_TEST_MAILPIT_PORT=32101 GEOGUESSME_TEST_DB_PORT=32102 \
        GEOGUESSME_TEST_TOXIPROXY_PORT=32103 GEOGUESSME_TEST_PORT_BASE=32100 \
        GEOGUESSME_E2E_PROJECTS=desktop GEOGUESSME_E2E_SHARD='' GEOGUESSME_E2E_SPEC='' \
        bash "$TMP/fixture/tools/quality/$helper" >"$TMP/output" 2>&1 || status=$?
    [ "$status" = 23 ] || {
        printf '%s\n' "$(<"$TMP/output")" >&2
        fail "$helper returned $status instead of fake tool status"
    }
    local project
    project="$(sed -n 's/^app|\([^|]*\)|.*/\1/p' "$trace" | sort -u)"
    if [ -z "$project" ] || [[ "$project" == *$'\n'* ]]; then
        fail "$helper startup/diagnostics/teardown used different projects"
    fi
    grep -Fq "operation|$project|" "$trace" || fail "$helper omitted app stack operations"
    grep -Fq ' down -v --remove-orphans' "$trace" || fail "$helper omitted isolated teardown"
    grep -Fxq "app|$project|32100|32101|32102|32103" "$trace" || fail "$helper did not retain caller port overrides"
    printf '%s' "$project"
}
for helper in run-integration.sh run-e2e.sh; do
    family=integration
    [ "$helper" != run-e2e.sh ] || family=e2e
    first="$(run_case "$helper" geoguessme-tools-checkout-one '' "$TMP/first-$family")"
    second="$(run_case "$helper" geoguessme-tools-checkout-one '' "$TMP/second-$family")"
    other="$(run_case "$helper" geoguessme-tools-checkout-two '' "$TMP/other-$family")"
    [[ "$first" =~ ^geoguessme-tools-checkout-one-$family-[0-9]+$ ]] || fail "$helper default lacks checkout/PID scope"
    [ "$first" != "$second" ] || fail "$helper reused a project across invocations"
    [[ "$other" =~ ^geoguessme-tools-checkout-two-$family-[0-9]+$ ]] || fail "$helper ignored another checkout namespace"
    custom="$(run_case "$helper" geoguessme-tools-checkout-one caller-project "$TMP/custom-$family")"
    [ "$custom" = caller-project ] || fail "$helper ignored caller project override"
    echo "PASS: $helper has per-checkout/run projects, isolated cleanup, and caller project/port overrides"
done

echo 'runner-isolation regression PASSED'
