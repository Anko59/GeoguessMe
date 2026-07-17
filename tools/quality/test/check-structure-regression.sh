#!/usr/bin/env bash
set -euo pipefail

CHECKER="$(cd "$(dirname "$0")/.." && pwd)/structure-check"
PASS=0
FAIL=0

with_temp_repo() {
    local expected=$1
    local name=$2
    shift 2
    local directory
    directory=$(mktemp -d)
    local status=0
    (
        cd "$directory"
        git init -q
        git config user.email test@localhost
        git config user.name test
        "$@"
        git add -A
        git commit -q -m fixture
        STRUCTURE_REPO_ROOT="$directory" "$CHECKER" >/dev/null
    ) || status=$?
    rm -rf "$directory"
    if [ "$expected" = pass ] && [ "$status" -eq 0 ]; then
        PASS=$((PASS + 1))
    elif [ "$expected" = fail ] && [ "$status" -ne 0 ]; then
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name"
        FAIL=$((FAIL + 1))
    fi
}

echo "structure-check regression tests:"
if (cd "$(dirname "$CHECKER")/../.." && "$CHECKER" >/dev/null); then
    PASS=$((PASS + 1))
else
    echo "FAIL: current-repository"
    FAIL=$((FAIL + 1))
fi
# shellcheck disable=SC2016
with_temp_repo pass exact-500 bash -c 'for i in $(seq 1 500); do echo line; done > exact.md'
# shellcheck disable=SC2016
with_temp_repo fail 501-lines bash -c 'for i in $(seq 1 501); do echo line; done > too-long.md'
# shellcheck disable=SC2016
with_temp_repo pass spaces-and-14-children bash -c 'mkdir -p "space dir"; for i in $(seq 1 14); do echo x > "space dir/f$i.ts"; done'
# shellcheck disable=SC2016
with_temp_repo fail 15-children bash -c 'mkdir -p too-many; for i in $(seq 1 15); do echo x > "too-many/f$i.ts"; done'
with_temp_repo pass root-special-files bash -c 'printf "Dockerfile\n" > Dockerfile; printf "x\n" > Makefile'
# shellcheck disable=SC2016
with_temp_repo pass generated-lock bash -c 'for i in $(seq 1 501); do echo x; done > package-lock.json'
# shellcheck disable=SC2016
with_temp_repo pass binary-media bash -c 'for i in $(seq 1 501); do echo x; done > image.png'

echo "results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
