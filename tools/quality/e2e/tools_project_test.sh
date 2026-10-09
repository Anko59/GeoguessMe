#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/frontend" "$fixture/tools/quality/e2e" \
    "$fixture/tools/make" "$fixture/deployment/scripts" "$fixture/volumes"
cp "$ROOT/tools/quality/run-e2e.sh" "$fixture/tools/quality/"
cp "$ROOT/tools/quality/e2e/arguments.sh" "$fixture/tools/quality/e2e/"
cp "$ROOT/tools/make/setup.mk" "$fixture/tools/make/"
printf '#!/usr/bin/env bash\nexit 0\n' >"$fixture/deployment/scripts/wait-for-health.sh"
chmod +x "$fixture/deployment/scripts/wait-for-health.sh"
cat >"$fixture/Makefile" <<'MAKE'
include tools/make/setup.mk
.PHONY: fixture-e2e
fixture-e2e:
	bash tools/quality/run-e2e.sh
MAKE
cat >"$fixture/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
project='' tools=false run=false bootstrap=false browser=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        -p) project=$2; shift ;;
        -f) [ "$2" != deployment/compose.tools.yaml ] || tools=true; shift ;;
        run) run=true ;;
        node-tools) bootstrap=true ;;
        playwright) browser=true ;;
    esac
    shift
done
if ! $tools || ! $run; then
    exit 0
fi
[[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || exit 2
volume="$E2E_VOLUME_ROOT/${project}_frontend-node-modules"
if $bootstrap; then
    printf 'locked dependencies\n' >"$volume"
    printf 'bootstrap %s\n' "$volume" >>"$E2E_DOCKER_TRACE"
elif $browser; then
    # This models consuming the existing Compose-scoped volume, not installing
    # dependencies or silently falling back to a different project's volume.
    test -f "$volume"
    test "$(<"$volume")" = 'locked dependencies'
    printf 'browser %s\n' "$volume" >>"$E2E_DOCKER_TRACE"
fi
MOCK
chmod +x "$fixture/bin/docker"
export PATH="$fixture/bin:$PATH" E2E_VOLUME_ROOT="$fixture/volumes"
export GEOGUESSME_E2E_PROJECTS=desktop GEOGUESSME_E2E_SPEC='' GEOGUESSME_E2E_SHARD=''
# Do not inherit the caller's Make command-line overrides into the fixture.
unset GEOGUESSME_TOOLS_PROJECT MAKEFLAGS MFLAGS MAKEOVERRIDES

default_project="geoguessme-tools-$(printf '%s' "$fixture" | cksum | awk '{print $1}')"
for project in "$default_project" geoguessme-tools-isolated_42; do
    export E2E_DOCKER_TRACE="$fixture/$project.trace"
    args=()
    if [ "$project" != "$default_project" ]; then
        args=("GEOGUESSME_TOOLS_PROJECT=$project")
    fi
    make --no-print-directory -C "$fixture" bootstrap-e2e "${args[@]}"
    make --no-print-directory -C "$fixture" fixture-e2e "${args[@]}"
    expected="$fixture/volumes/${project}_frontend-node-modules"
    printf 'bootstrap %s\nbrowser %s\n' "$expected" "$expected" >"$fixture/expected"
    diff -u "$fixture/expected" "$E2E_DOCKER_TRACE"
    # Standalone execution must consume the same existing volume as Make.
    GEOGUESSME_TOOLS_PROJECT="$project" GEOGUESSME_TEST_WEB_PORT=32100 \
        GEOGUESSME_TEST_MAILPIT_PORT=32101 bash "$fixture/tools/quality/run-e2e.sh"
    printf 'browser %s\n' "$expected" >>"$fixture/expected"
    diff -u "$fixture/expected" "$E2E_DOCKER_TRACE"
    echo "PASS: tools project $project shares the existing bootstrap npm volume"
done

for project in '' 'Uppercase' '-leading' 'with space' 'bad;touch injected'; do
    status=0
    GEOGUESSME_TOOLS_PROJECT="$project" bash "$fixture/tools/quality/run-e2e.sh" \
        >"$fixture/invalid.output" 2>&1 || status=$?
    test "$status" -eq 2
    grep -q 'GEOGUESSME_TOOLS_PROJECT must match' "$fixture/invalid.output"
done
echo 'PASS: invalid tools project names fail before stack or artifact mutation'
