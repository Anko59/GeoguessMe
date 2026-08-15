#!/usr/bin/env bash
set -euo pipefail

checker=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-markers.sh
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
git_env=(env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_COMMON_DIR -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES)

fixture_git() {
    "${git_env[@]}" git -C "$fixture" "$@"
}

run_checker() {
    "${git_env[@]}" "$checker" "$fixture"
}

fixture_git init -q
fixture_git config user.email test@example.invalid
fixture_git config user.name Test

write_fixture() {
    printf '%s\n' "$2" >"$fixture/example.ts"
    fixture_git add example.ts
}

expect_pass() {
    write_fixture "$1" "$2"
    run_checker >/dev/null
    printf 'PASS: %s\n' "$1"
}

expect_fail() {
    write_fixture "$1" "$2"
    if run_checker >/dev/null 2>&1; then
        printf 'FAIL: %s unexpectedly passed\n' "$1" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$1"
}

expect_pass issue-reference '// TODO(#123): replace after rollout'
expect_pass issue-url '// FIXME(https://github.com/example/repo/issues/456): remove adapter'
expect_pass ledger-reference '// HACK(docs/agent-engineering.md#compatibility-ledger): legacy path'
expect_pass ordinary-word '// This todo list is runtime data.'
expect_fail bare-marker '// TODO: clean this up'
expect_fail mixed-owned-and-bare '// TODO(#123): first; FIXME: second'

echo 'debt marker regression tests PASSED'
