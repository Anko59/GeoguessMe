#!/usr/bin/env bash
set -euo pipefail

checker=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-markers.sh
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

git -C "$fixture" init -q
git -C "$fixture" config user.email test@example.invalid
git -C "$fixture" config user.name Test

write_fixture() {
    printf '%s\n' "$2" >"$fixture/example.ts"
    git -C "$fixture" add example.ts
}

expect_pass() {
    write_fixture "$1" "$2"
    "$checker" "$fixture" >/dev/null
    printf 'PASS: %s\n' "$1"
}

expect_fail() {
    write_fixture "$1" "$2"
    if "$checker" "$fixture" >/dev/null 2>&1; then
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
