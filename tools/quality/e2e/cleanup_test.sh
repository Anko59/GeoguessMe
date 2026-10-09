#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/frontend" "$fixture/tools/quality/e2e" "$fixture/deployment/scripts"
cp "$ROOT/tools/quality/run-e2e.sh" "$fixture/tools/quality/"
cp "$ROOT/tools/quality/e2e/arguments.sh" "$fixture/tools/quality/e2e/"
foreign_stage="$fixture/frontend/.playwright-run-foreign.active"
mkdir -p "$foreign_stage"
printf 'active browser work\n' >"$foreign_stage/sentinel"
printf '#!/usr/bin/env bash\nexit 0\n' >"$fixture/deployment/scripts/wait-for-health.sh"
chmod +x "$fixture/deployment/scripts/wait-for-health.sh"
cat >"$fixture/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$E2E_DOCKER_TRACE"
case "$*" in
    *" up -d --wait") exit "$E2E_UP_STATUS" ;;
    *" logs --no-color "*)
        echo 'migration startup diagnostic'
        exit "$E2E_LOG_STATUS"
        ;;
    *" down -v "*) exit "$E2E_TEARDOWN_STATUS" ;;
    *" playwright node "*)
        args=("$@")
        stage=""
        for ((i = 0; i < ${#args[@]}; i++)); do
            if [ "${args[i]}" = -v ] && [[ "${args[i + 1]:-}" == *:/tmp/playwright ]]; then
                stage="${args[i + 1]%:/tmp/playwright}"
            fi
        done
        [ -n "$stage" ] || exit 99
        mkdir -p "$stage/test-results"
        printf 'browser diagnostic' >"$stage/test-results/evidence.txt"
        exit "$E2E_BROWSER_STATUS"
        ;;
esac
MOCK
chmod +x "$fixture/bin/docker"

run_case() {
    local name=$1 startup=$2 browser=$3 teardown=$4 logs=$5 expected=$6 status=0
    local trace="$fixture/$name.trace" output="$fixture/$name.output"
    PATH="$fixture/bin:$PATH" E2E_DOCKER_TRACE="$trace" \
        E2E_UP_STATUS="$startup" E2E_BROWSER_STATUS="$browser" \
        E2E_TEARDOWN_STATUS="$teardown" E2E_LOG_STATUS="$logs" \
        GEOGUESSME_TOOLS_PROJECT=fixture-tools \
        GEOGUESSME_TEST_WEB_PORT=32100 GEOGUESSME_TEST_MAILPIT_PORT=32101 \
        GEOGUESSME_TEST_PROJECT=fixture GEOGUESSME_E2E_PROJECTS=desktop \
        GEOGUESSME_E2E_SPEC='' GEOGUESSME_E2E_SHARD='' \
        bash "$fixture/tools/quality/run-e2e.sh" >"$output" 2>&1 || status=$?
    if [ "$status" -ne "$expected" ]; then
        cat "$output" >&2
        echo "FAIL: $name returned $status, expected $expected" >&2
        exit 1
    fi
    if [ "$startup" -ne 0 ] || [ "$browser" -ne 0 ]; then
        grep -q 'migration startup diagnostic' "$output"
        awk '/logs --no-color/ { logs = NR } /down -v/ { down = NR }
            END { exit !(logs > 0 && down > logs) }' "$trace"
    else
        if grep -q 'logs --no-color' "$trace"; then
            echo 'FAIL: successful runs must not dump failure diagnostics' >&2
            exit 1
        fi
        grep -q 'down -v' "$trace"
    fi
    if [ "$startup" -eq 0 ]; then
        grep -q 'browser diagnostic' "$fixture/frontend/test-results/evidence.txt"
    fi
    grep -Fxq 'active browser work' "$foreign_stage/sentinel"
    for stage in "$fixture"/frontend/.playwright-run-*; do
        [ "$stage" = "$foreign_stage" ] || {
            echo "FAIL: $name left its own staging directory behind" >&2
            exit 1
        }
    done
    echo "PASS: E2E cleanup $name preserves status, diagnostics, teardown, and another run's staging"
}

run_case startup-failure 42 0 0 0 42
run_case browser-failure 0 71 0 0 71
run_case diagnostics-failure 42 0 0 23 42
run_case cleanup-after-failure 42 0 19 0 42
run_case cleanup-failure 0 0 19 0 19
run_case success 0 0 0 0 0

bash "$ROOT/tools/quality/e2e/runner_isolation_test.sh"
