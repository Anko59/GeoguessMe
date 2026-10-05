#!/usr/bin/env bash
# Sourced by the production-container regression suite.

# ── Test 16: Effective port merge and failed-start cleanup ────────────────────
echo "--- Test 16: Effective Compose port bindings and lifecycle ---"
# Only real Compose config is allowed through this shim. Image inspection and
# ELF checks are fixtures; up fails deliberately and down never reaches Docker.
# This exercises the script's actual generated override without starting a stack
# or performing operations on production/disposable data.
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
export PROD_VERIFY_REAL_DOCKER
PROD_VERIFY_REAL_DOCKER=$(command -v docker)
cat >"$fixture/docker" <<'DOCKEREOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
    image)
        if [ "$2" != inspect ]; then exit 90; fi
        if [ "${3:-}" != --format ]; then exit 0; fi
        case "$4" in
            '{{.Id}}') printf 'sha256:%064d\n' 1 ;;
            '{{.Config.User}}') echo 65532:65532 ;;
            *Healthcheck*) echo '[CMD healthcheck]' ;;
            '{{.Architecture}}') echo amd64 ;;
            *) exit 90 ;;
        esac
        ;;
    run)
        echo '62 0'
        ;;
    inspect)
        test "${4:-}" = fixture-oauth2-proxy
        printf 'health %s\n' "$4" >>"$PROD_VERIFY_CALLS"
        echo 'fixture-oauth2-proxy status=restarting health={"Status":"unhealthy"}'
        ;;
    compose)
        args=("$@")
        override=""
        project=""
        operation=""
        while [ "$#" -gt 0 ]; do
            case "$1" in
                -f)
                    if [[ "$2" == */override.yaml ]]; then override="$2"; fi
                    shift
                    ;;
                -p) project="$2"; shift ;;
                config | up | down | ps | logs) operation="$1" ;;
            esac
            shift
        done
        case "$operation" in
            config)
                exec "$PROD_VERIFY_REAL_DOCKER" "${args[@]}"
                ;;
            up)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                printf 'up %s %s\n' "$project" "$override" >>"$PROD_VERIFY_CALLS"
                # Replace the requested up command with a config-only render.
                config_args=()
                for arg in "${args[@]}"; do
                    if [ "$arg" = up ]; then break; fi
                    config_args+=("$arg")
                done
                "$PROD_VERIFY_REAL_DOCKER" "${config_args[@]}" config >"$PROD_VERIFY_CONFIG"
                exit 73
                ;;
            ps | logs)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                printf '%s %s %s\n' "$operation" "$project" "$override" >>"$PROD_VERIFY_CALLS"
                if [[ " ${args[*]} " == *' ps -aq '* ]]; then
                    echo fixture-oauth2-proxy
                else
                    echo "fixture $operation diagnostic"
                fi
                ;;
            down)
                test "$project" = "$GEOGUESSME_PROD_VERIFY_PROJECT"
                test -f "$override"
                [[ " ${args[*]} " == *' down -v --remove-orphans '* ]]
                printf 'down %s %s\n' "$project" "$override" >>"$PROD_VERIFY_CALLS"
                if [ "${PROD_VERIFY_DOWN_FAIL:-0}" = 1 ]; then
                    echo 'simulated down failure diagnostic' >&2
                    exit 42
                fi
                ;;
            *) exit 90 ;;
        esac
        ;;
    *) exit 90 ;;
esac
DOCKEREOF
chmod +x "$fixture/docker"

printf 'FORBIDDEN_PRODUCTION_VALUE=fake-private-sentinel\n' >"$fixture/inherited.env"
for scenario in default custom; do
    expected_port=18083
    base_port=8081
    requested_port=""
    if [ "$scenario" = custom ]; then
        expected_port=19083
        base_port=19081
        requested_port="$expected_port"
    fi
    export PROD_VERIFY_CALLS="$fixture/$scenario.calls"
    export PROD_VERIFY_CONFIG="$fixture/$scenario.config"
    project="geoguessme-prod-verify-regression-$scenario"
    status=0
    PATH="$fixture:$PATH" GEOGUESSME_PROD_VERIFY_PROJECT="$project" \
        GEOGUESSME_TEST_PORT_BASE=18079 GEOGUESSME_PROD_VERIFY_WEB_PORT="$requested_port" \
        GEOGUESSME_WEB_PORT="$base_port" \
        GEOGUESSME_ENV_FILE="$fixture/inherited.env" \
        bash "$SCRIPT" >"$fixture/$scenario.log" 2>&1 || status=$?
    if [ "$status" -ne 73 ]; then
        fail "$scenario: expected simulated startup failure (73), got $status"
        cat "$fixture/$scenario.log"
        continue
    fi
    # Compose's normalized YAML exposes every effective binding, rather than
    # merely checking that the source override contains the desired port.
    bindings=$(awk '
        /^  web:$/ { web = 1; next }
        web && /^  [[:alnum:]_-]+:$/ { exit }
        web && /^    ports:$/ { ports = 1; next }
        ports && /^    [[:alnum:]_-]+:/ { ports = 0 }
        ports && /host_ip:|target:|published:|protocol:/ {
            gsub(/"/, ""); print $1, $2
        }
    ' "$PROD_VERIFY_CONFIG")
    expected=$(printf 'host_ip: 127.0.0.1\ntarget: 80\npublished: %s\nprotocol: tcp' "$expected_port")
    if [ "$bindings" = "$expected" ]; then
        pass "$scenario: exactly one loopback gateway binding, replaces inherited $base_port"
    else
        fail "$scenario: effective gateway bindings differ from requested loopback-only port"
        printf '%s\n' "$bindings"
    fi
    override=$(awk '$1 == "up" { print $3 }' "$PROD_VERIFY_CALLS")
    expected_calls=$(printf 'up %s %s\nps %s %s\nlogs %s %s\nps %s %s\nhealth fixture-oauth2-proxy\ndown %s %s' \
        "$project" "$override" "$project" "$override" "$project" "$override" \
        "$project" "$override" "$project" "$override")
    if grep -q 'fixture logs diagnostic' "$fixture/$scenario.log" &&
        grep -q 'health={"Status":"unhealthy"}' "$fixture/$scenario.log" &&
        ! grep -q 'FORBIDDEN_PRODUCTION_VALUE' "$PROD_VERIFY_CONFIG"; then
        pass "$scenario: fixture-only logs/status/health emitted before teardown"
    else
        fail "$scenario: diagnostic missing or inherited env_file retained"
    fi
    if [ "$(<"$PROD_VERIFY_CALLS")" = "$expected_calls" ] &&
        [ -n "$override" ] && [ ! -e "$(dirname "$override")" ]; then
        pass "$scenario: failed up tears down only managed project and removes temp files"
    else
        fail "$scenario: failed-start lifecycle/temporary-file cleanup differs"
    fi
done

# shellcheck source=tools/quality/test/prod-container-verify/cleanup-regression.sh
source "$(dirname "$0")/prod-container-verify/cleanup-regression.sh"
