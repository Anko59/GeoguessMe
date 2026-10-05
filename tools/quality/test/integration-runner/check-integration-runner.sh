#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../../.." && pwd -P)
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
cat >"$fixture/git" <<'GIT'
#!/bin/sh
printf '1111111111111111111111111111111111111111\n'
GIT
cat >"$fixture/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
state=$(<"$INTEGRATION_STATE")
case "$1" in
    ps)
        [[ "$*" == *"label=com.docker.compose.project=$INTEGRATION_PROJECT"* ]] || exit 90
        if [ "$state" = replacement ]; then echo replacement-backend
        elif [ "$state" != empty ]; then echo fixture-backend; fi
        exit 0
        ;;
    image)
        test "${5:-}" = geoguessme-backend:local-private
        echo sha256:expected
        exit 0
        ;;
    inspect)
        case "$3" in
            *working_dir*)
                if [ "$state" = foreign ]; then echo "$INTEGRATION_ROOT/foreign"; else echo "$INTEGRATION_ROOT"; fi
                ;;
            *org.opencontainers.image.revision*)
                if [ "$INTEGRATION_CASE" = wrong-revision ]; then echo wrong-revision; else echo '<no value>'; fi
                ;;
            '{{.Id}} {{.Image}}')
                if [ "$state" = replacement ]; then echo 'replacement-backend sha256:expected'
                elif [ "$state" = changed-image ] || [ "$state" = wrong-image ]; then echo 'fixture-backend sha256:changed'
                else echo 'fixture-backend sha256:expected'; fi
                ;;
            '{{.Image}}')
                if [ "$state" = wrong-image ]; then echo sha256:changed; else echo sha256:expected; fi
                ;;
            *) exit 90 ;;
        esac
        exit 0
        ;;
    compose) ;;
    *) exit 90 ;;
esac
args="$*"; project=''; operation=''
while [ "$#" -gt 0 ]; do
    case "$1" in
        -p) project=$2; shift ;;
        up | run | ps | logs | down) operation=$1 ;;
    esac
    shift
done
# Project-selected destructive/diagnostic operations must never see foreign
# source ownership or a different invocation's container/image identities.
if [ "$operation" = logs ] || [ "$operation" = down ] || [ "$operation" = ps ]; then
    case "$state" in foreign | replacement | changed-image) echo 'FOREIGN OPERATION' >&2; exit 91 ;; esac
fi
if [ "$operation" = run ]; then
    if [[ "$args" == *'go test ./integration_test -count=1'* ]]; then
        test "$project" = "$INTEGRATION_TOOLS"
        [[ "$args" == *':28480'* && "$args" == *':28425'* && "$args" == *':25432'* && "$args" == *':28474'* ]]
        echo tests >>"$INTEGRATION_CALLS"
        case "$INTEGRATION_CASE" in
            foreign-after-test | failed-test-takeover) echo foreign >"$INTEGRATION_STATE" ;;
            id-after-test) echo replacement >"$INTEGRATION_STATE" ;;
            image-after-test) echo changed-image >"$INTEGRATION_STATE" ;;
        esac
        if [ "$INTEGRATION_CASE" = failed-test-takeover ] || [ "$INTEGRATION_CASE" = failed-test ]; then exit 73; fi
    else
        # Existing health helper uses its own legacy tooling-project default.
        echo health >>"$INTEGRATION_CALLS"
    fi
    exit 0
fi
test "$project" = "$INTEGRATION_PROJECT"
echo "$operation" >>"$INTEGRATION_CALLS"
case "$operation" in
    up)
        test "$GEOGUESSME_TEST_WEB_PORT" = 28480
        test "$GEOGUESSME_TEST_MAILPIT_PORT" = 28425
        test "$GEOGUESSME_TEST_DB_PORT" = 25432
        test "$GEOGUESSME_TEST_TOXIPROXY_PORT" = 28474
        case "$INTEGRATION_CASE" in
            foreign-after-up) echo foreign >"$INTEGRATION_STATE" ;;
            wrong-image) echo wrong-image >"$INTEGRATION_STATE" ;;
            *) echo owned >"$INTEGRATION_STATE" ;;
        esac
        ;;
    ps) if [[ "$args" == *'ps -q backend'* ]]; then echo fixture-backend; fi ;;
    logs) echo 'owned fake fixture logs' ;;
    down)
        if [ "$INTEGRATION_CASE" = teardown-failure ]; then exit 42; fi
        echo empty >"$INTEGRATION_STATE"
        ;;
    *) exit 90 ;;
esac
DOCKER
chmod +x "$fixture/docker" "$fixture/git"
for scenario in success existing-own foreign-start foreign-after-up foreign-after-test failed-test-takeover \
    id-after-test image-after-test wrong-image wrong-revision failed-test teardown-failure; do
    state="$fixture/$scenario.state"
    calls="$fixture/$scenario.calls"
    echo empty >"$state"
    : >"$calls"
    [ "$scenario" != existing-own ] || echo owned >"$state"
    [ "$scenario" != foreign-start ] || echo foreign >"$state"
    status=0
    PATH="$fixture:$PATH" INTEGRATION_ROOT="$root" INTEGRATION_STATE="$state" INTEGRATION_CALLS="$calls" \
        INTEGRATION_CASE="$scenario" INTEGRATION_PROJECT=geoguessme-private-integration \
        INTEGRATION_TOOLS=geoguessme-private-tools GEOGUESSME_TEST_PROJECT=geoguessme-private-integration \
        GEOGUESSME_TOOLS_PROJECT=geoguessme-private-tools GEOGUESSME_TEST_WEB_PORT=28480 \
        GEOGUESSME_TEST_MAILPIT_PORT=28425 GEOGUESSME_TEST_DB_PORT=25432 \
        GEOGUESSME_TEST_TOXIPROXY_PORT=28474 BACKEND_IMAGE=geoguessme-backend:local-private \
        bash "$root/tools/quality/run-integration.sh" >"$fixture/$scenario.log" 2>&1 || status=$?
    expected=1
    case "$scenario" in
        success) expected=0 ;;
        failed-test | failed-test-takeover) expected=73 ;;
        teardown-failure) expected=42 ;;
    esac
    test "$status" -eq "$expected"
    if grep -q 'FOREIGN OPERATION' "$fixture/$scenario.log"; then exit 1; fi
    case "$scenario" in
        foreign-* | failed-test-takeover | id-after-test | image-after-test | existing-own)
            if grep -Eq '^(logs|down)$' "$calls"; then exit 1; fi
            ;;
        *) grep -q '^down$' "$calls" ;;
    esac
    case "$scenario" in
        foreign-start | existing-own) test ! -s "$calls" ;;
        wrong-image | wrong-revision | foreign-after-up)
            if grep -q '^tests$' "$calls"; then exit 1; fi
            ;;
    esac
done
echo 'integration runner regression PASSED: private ports/tooling, startup ownership, image/revision checks, takeover/ID/image mutation refusal and exit status'
