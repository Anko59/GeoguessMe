#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
cat >"$fixture/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == compose ]] || exit 90
args="$*"; project=''; operation=''
while [ "$#" -gt 0 ]; do
    case "$1" in
        -p) project=$2; shift ;;
        up | run | ps | logs | down) operation=$1 ;;
    esac
    shift
done
printf '%s %s\n' "$operation" "$project" >>"$LOAD_CALLS"
if [ "$operation" = run ]; then
    test "$project" = "$LOAD_EXPECT_TOOLS_PROJECT"
    [[ "$args" == *'run -T --rm --no-deps --user 1000:1000 loadtest k6 run'* ]]
    [[ "$args" == *"BASE_URL=http://host.docker.internal:$LOAD_EXPECT_PORT"* ]]
    [[ "$args" == *'VUS=5'* && "$args" == *'DURATION=30s'* && "$args" == *'/workspace/tools/load/k6.js'* ]]
    exit "${LOAD_K6_STATUS:-0}"
fi
test "$project" = "$LOAD_EXPECT_PROJECT"
test "$GEOGUESSME_TEST_WEB_PORT" = "$LOAD_EXPECT_PORT"
test "$GEOGUESSME_TEST_PUBLIC_URL" = "http://localhost:$LOAD_EXPECT_PORT"
case "$operation" in
    up) exit "${LOAD_UP_STATUS:-0}" ;;
    ps) echo 'fake load fixture status' ;;
    logs) echo 'fake load fixture logs' ;;
    down)
        [[ "$args" == *'down -v --remove-orphans'* ]]
        exit "${LOAD_DOWN_STATUS:-0}"
        ;;
    *) exit 90 ;;
esac
DOCKER
chmod +x "$fixture/docker"
for scenario in default custom k6-failure startup-failure teardown-failure; do
    project=geoguessme-load
    tools_project=geoguessme-tools
    port=18080
    requested_project=''
    requested_tools=''
    requested_port=''
    k6_status=0
    up_status=0
    down_status=0
    expected_status=0
    if [ "$scenario" = custom ]; then
        project=geoguessme-load-private
        tools_project=geoguessme-issues-integration-tools
        port=19080
        requested_project=$project
        requested_tools=$tools_project
        requested_port=$port
    fi
    case "$scenario" in
        k6-failure)
            k6_status=99
            down_status=42
            expected_status=99
            ;;
        startup-failure)
            up_status=73
            expected_status=73
            ;;
        teardown-failure)
            down_status=42
            expected_status=42
            ;;
    esac
    calls="$fixture/$scenario.calls"
    status=0
    PATH="$fixture:$PATH" TOOLS_UID=1000 TOOLS_GID=1000 LOAD_VUS=5 LOAD_DURATION=30s \
        GEOGUESSME_LOAD_PROJECT="$requested_project" GEOGUESSME_TOOLS_PROJECT="$requested_tools" \
        GEOGUESSME_TEST_WEB_PORT="$requested_port" LOAD_EXPECT_PORT="$port" \
        LOAD_EXPECT_PROJECT="$project" LOAD_EXPECT_TOOLS_PROJECT="$tools_project" LOAD_CALLS="$calls" \
        LOAD_K6_STATUS="$k6_status" LOAD_UP_STATUS="$up_status" LOAD_DOWN_STATUS="$down_status" \
        bash "$root/deployment/scripts/load-test.sh" >"$fixture/$scenario.log" 2>&1 || status=$?
    test "$status" -eq "$expected_status"
    expected=$(printf 'up %s\nrun %s\ndown %s' "$project" "$tools_project" "$project")
    if [ "$scenario" = k6-failure ]; then
        expected=$(printf 'up %s\nrun %s\nps %s\nlogs %s\ndown %s' "$project" "$tools_project" "$project" "$project" "$project")
    fi
    if [ "$scenario" = startup-failure ]; then
        expected=$(printf 'up %s\nps %s\nlogs %s\ndown %s' "$project" "$project" "$project" "$project")
    fi
    test "$(<"$calls")" = "$expected"
done
# Root/named identities must fail before starting or tearing down any stack.
for uid in 0 root invalid; do
    calls="$fixture/rejected-$uid.calls"
    status=0
    PATH="$fixture:$PATH" TOOLS_UID="$uid" TOOLS_GID=1000 LOAD_CALLS="$calls" \
        bash "$root/deployment/scripts/load-test.sh" >"$fixture/rejected-$uid.log" 2>&1 || status=$?
    test "$status" -eq 2
    test ! -e "$calls"
    grep -q 'requires a numeric non-root TOOLS_UID' "$fixture/rejected-$uid.log"
done
echo 'load runner regression PASSED: nonroot owner mapping, default/private projects, unchanged profile arguments and primary/cleanup status'
