#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck disable=SC1091 # Runtime-relative source resolved above.
source "$script_dir/arguments.sh"

assert_args() {
    local name=$1
    local expected=$2
    shift 2
    local -a actual
    build_e2e_test_args actual "$@"
    local rendered
    printf -v rendered '%s\n' "${actual[@]}"
    rendered=${rendered%$'\n'}
    if [ "$rendered" != "$expected" ]; then
        printf 'FAIL: %s\nexpected:\n%s\nactual:\n%s\n' "$name" "$expected" "$rendered" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

assert_fails() {
    local name=$1
    shift
    local -a actual
    if build_e2e_test_args actual "$@" >/dev/null 2>&1; then
        printf 'FAIL: %s unexpectedly succeeded\n' "$name" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

assert_args "default matrix" $'test\n--project=desktop\n--project=firefox\n--project=mobile' \
    'desktop,firefox,mobile' '' '' ''
# shellcheck disable=SC2016 # Literal metacharacters must remain unexpanded.
assert_args "CI shard and metacharacter-safe spec" $'test\n--project=desktop\n--shard=2/3\npath with $(literal).spec.ts' \
    desktop '2/3' 'path with $(literal).spec.ts' ''
assert_args "UI mode" $'test\n--project=desktop\n--ui' desktop '' '' --ui

assert_fails "zero shard" desktop '0/2' '' ''
assert_fails "shard exceeds total" desktop '3/2' '' ''
assert_fails "malformed shard" desktop 'first/second' '' ''
assert_fails "unsupported project" webkit '' '' ''
assert_fails "UI shard conflict" desktop '1/2' '' --ui
assert_fails "unknown runner argument" desktop '' '' --debug

echo "E2E argument regression tests PASSED"
