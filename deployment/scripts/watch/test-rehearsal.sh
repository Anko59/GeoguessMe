#!/bin/sh
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
# shellcheck source=deployment/scripts/watch/prepare-rehearsal.sh
. "$ROOT/deployment/scripts/watch/prepare-rehearsal.sh"
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT INT TERM
mkdir -p "$fixture/checkout/deployment/watch" "$fixture/staged" "$fixture/bin"
chmod 0755 "$fixture/staged"
for name in Caddyfile vector.yaml victoria-metrics.yaml; do
    cp "$ROOT/deployment/watch/$name" "$fixture/checkout/deployment/watch/$name"
    chmod 0600 "$fixture/checkout/deployment/watch/$name"
done
printf 'fake-private-sentinel\n' >"$fixture/checkout/private.env"
chmod 0600 "$fixture/checkout/private.env"
(
    umask 077
    prepare_watch_fixture "$fixture/checkout" "$fixture/staged" fixture-production fixture-development
)
for name in Caddyfile vector.yaml victoria-metrics.yaml; do
    test "$(stat -c %a "$fixture/staged/public/$name")" = 644
    test "$(stat -c %a "$fixture/checkout/deployment/watch/$name")" = 600
done
cmp "$ROOT/deployment/watch/Caddyfile" "$fixture/staged/public/Caddyfile"
cmp "$ROOT/deployment/watch/victoria-metrics.yaml" "$fixture/staged/public/victoria-metrics.yaml"
sed '/include_containers:/d' "$fixture/staged/public/vector.yaml" >"$fixture/vector-original"
cmp "$ROOT/deployment/watch/vector.yaml" "$fixture/vector-original"
grep -Fq 'include_containers: ["fixture-production", "fixture-development"]' "$fixture/staged/public/vector.yaml"
test "$(stat -c %a "$fixture/staged/public")" = 755
test "$(stat -c %a "$fixture/staged/agent.env")" = 600
test "$(stat -c %a "$fixture/checkout/private.env")" = 600
for name in production-metrics-token mock.Caddyfile metrics; do
    test "$(stat -c %a "$fixture/staged/$name")" = 644
done
test "$(grep -c 'volumes: !override' "$fixture/staged/override.yaml")" -eq 3
grep -q 'env_file: !override' "$fixture/staged/override.yaml"
# Public input symlinks must not leak private files into readable staging.
path="$fixture/checkout/deployment/watch/vector.yaml"
test "$path" = "$fixture/checkout/deployment/watch/vector.yaml"
rm "$path"
ln -s "$fixture/checkout/private.env" "$path"
mkdir "$fixture/rejected"
if prepare_watch_fixture "$fixture/checkout" "$fixture/rejected" fixture-production fixture-development; then
    echo 'watch fixture accepted a symlink to a private file' >&2
    exit 1
fi
test "$(stat -c %a "$fixture/checkout/private.env")" = 600

# Run the real rehearsal script against a fake Docker CLI. No host socket,
# containers, ports, networks, production credentials or unconditional waits.
cat >"$fixture/bin/docker" <<'DOCKER'
#!/bin/sh
set -eu
case "$1" in
    image)
        test "$2" = inspect
        test "$5" = geoguessme-web:local-private
        printf 'sha256:%064d\n' 1
        ;;
    network)
        printf 'network %s\n' "$2" >>"$WATCH_CALLS"
        ;;
    run)
        printf 'mock run\n' >>"$WATCH_CALLS"
        ;;
    compose)
        project=''; override=''; operation=''; all="$*"
        while [ "$#" -gt 0 ]; do
            case "$1" in
                -p) project=$2; shift ;;
                -f) case "$2" in */override.yaml) override=$2 ;; esac; shift ;;
                up | ps | logs | down) operation=$1 ;;
            esac
            shift
        done
        case "$project" in geoguessme-watch-rehearsal-*) ;; *) exit 90 ;; esac
        test -f "$override"
        printf '%s %s %s\n' "$operation" "$project" "$override" >>"$WATCH_CALLS"
        case "$operation" in
            up) exit 73 ;;
            ps) case "$all" in *' ps -aq') echo fixture-watch ;; *) echo 'fixture status' ;; esac ;;
            logs)
                echo 'fixture config permission diagnostic'
                if [ "${WATCH_DIAGNOSTIC_FAIL:-0}" = 1 ]; then exit 44; fi
                ;;
            down) if [ "${WATCH_DOWN_FAIL:-0}" = 1 ]; then exit 42; fi ;;
            *) exit 90 ;;
        esac
        ;;
    inspect)
        test "$2" = --format
        test "$4" = fixture-watch
        case "$3" in *Config.Env*) exit 90 ;; esac
        printf 'health\n' >>"$WATCH_CALLS"
        echo 'fixture-watch status=restarting health={"Status":"unhealthy"}'
        ;;
    rm) printf 'mock rm\n' >>"$WATCH_CALLS" ;;
    *) exit 90 ;;
esac
DOCKER
chmod +x "$fixture/bin/docker"
for scenario in normal diagnostics-fail teardown-fail; do
    WATCH_CALLS="$fixture/$scenario.calls"
    export WATCH_CALLS
    diagnostic_fail=0
    down_fail=0
    [ "$scenario" != diagnostics-fail ] || diagnostic_fail=1
    [ "$scenario" != teardown-fail ] || down_fail=1
    status=0
    PATH="$fixture/bin:$PATH" WATCH_DIAGNOSTIC_FAIL="$diagnostic_fail" WATCH_DOWN_FAIL="$down_fail" \
        WEB_IMAGE=geoguessme-web:local-private GEOGUESSME_TOOLS_PROJECT=geoguessme-private-tools \
        GEOGUESSME_TEST_PORT_BASE=28000 sh "$ROOT/deployment/scripts/watch/rehearsal.sh" >"$fixture/$scenario.log" 2>&1 || status=$?
    test "$status" -eq 73
    grep -q 'fixture config permission diagnostic' "$fixture/$scenario.log"
    grep -q 'health={"Status":"unhealthy"}' "$fixture/$scenario.log"
    override=$(awk '$1 == "up" {print $3}' "$WATCH_CALLS")
    test -n "$override" && test ! -e "$(dirname "$override")"
    operations=$(awk '{print $1}' "$WATCH_CALLS" | tr '\n' ' ')
    test "$operations" = 'network mock mock up ps logs ps health down mock network '
done
# A teardown error must also fail a successful rehearsal, not just preserve an
# existing startup failure. Reuse the real functions without running a stack.
awk '
    /^compose\(\) \{|^cleanup\(\) \{/ { inside = 1 }
    inside { print }
    inside && /^}/ { inside = 0 }
' "$ROOT/deployment/scripts/watch/rehearsal.sh" >"$fixture/cleanup.sh"
mkdir "$fixture/success-cleanup"
: >"$fixture/success-cleanup/override.yaml"
WATCH_CALLS="$fixture/success-cleanup.calls"
export WATCH_CALLS ROOT
status=0
PATH="$fixture/bin:$PATH" WATCH_DOWN_FAIL=1 PROJECT=geoguessme-watch-rehearsal-cleanup \
    TMP="$fixture/success-cleanup" WATCH_COMPOSE="$ROOT/deployment/compose.watch.yaml" \
    MOCK_BACKEND=fixture-production MOCK_DEV=fixture-development NETWORK=fixture-network \
    sh -c '. "$1"; trap cleanup EXIT; exit 0' _ "$fixture/cleanup.sh" \
    >"$fixture/success-cleanup.log" 2>&1 || status=$?
test "$status" -eq 42
test ! -e "$fixture/success-cleanup"
grep -q 'watch rehearsal cleanup failed' "$fixture/success-cleanup.log"
printf 'watch rehearsal regression passed: restrictive checkout staging, fake-only mounts, private unchanged, bounded diagnostics and primary failure/cleanup\n'
