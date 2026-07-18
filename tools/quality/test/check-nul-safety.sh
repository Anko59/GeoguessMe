#!/usr/bin/env bash
set -euo pipefail

# Verify that the NUL-delimited file-listing patterns used in the
# Dockerized Makefile targets correctly handle filenames with spaces
# and shell metacharacters.

PASS=0
FAIL=0

assert_pass() {
    local name=$1 desc=$2 rc=$3
    if [ "$rc" -eq 0 ]; then
        echo "PASS: $name - $desc"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name - $desc (rc=$rc)"
        FAIL=$((FAIL + 1))
    fi
}

assert_find() {
    local name=$1 desc=$2
    shift 2
    local output
    output=$("$@" 2>/dev/null) || true
    if [ -n "$output" ]; then
        echo "PASS: $name - $desc"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name - $desc (no output)"
        FAIL=$((FAIL + 1))
    fi
}

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

cd "$workdir"
git init -q
git config user.email test@localhost
git config user.name test

# ── Create files with spaces, shell metacharacters, and quotes ───────────────
mkdir -p "dir with spaces/.github/workflows"

cat >"dir with spaces/simple script.sh" <<'SHEOF'
#!/bin/sh
echo "hello"
SHEOF
chmod +x "dir with spaces/simple script.sh"

cat >"dir with spaces/multi word.Dockerfile" <<'DOKOF'
FROM alpine:3.21
DOKOF

cat >"dir with spaces/.github/workflows/test workflow.yml" <<'WFEOF'
name: Test
on:
  push:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
WFEOF

git add -A
git commit -q -m fixture

# ── Test: find -print0 | sort -z | xargs -0 for shell scripts ──────────────
result=$(find . -type f -name '*.sh' \
    -not -path './.git/*' \
    -not -path './frontend/node_modules/*' \
    -not -path './frontend/coverage/*' \
    -print0 | sort -z | xargs -0 -r echo 2>/dev/null) || true
assert_pass "find-shell-print0" "finds .sh with spaces" \
    "$([ -n "$result" ] && echo 0 || echo 1)"

result=$(find . -type f \( -name 'Dockerfile' -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) \
    -not -path './.git/*' \
    -not -path './frontend/node_modules/*' \
    -print0 | sort -z | xargs -0 -r echo 2>/dev/null) || true
assert_pass "find-docker-print0" "finds Dockerfiles with spaces" \
    "$([ -n "$result" ] && echo 0 || echo 1)"

# ── Test: git ls-files -z | xargs -0 patterns ──────────────────────────────
result=$(git ls-files -z '*.sh' | xargs -0 -r echo 2>/dev/null) || true
assert_pass "git-ls-sh" "git ls-files -z *.sh survives spaces" \
    "$([ -n "$result" ] && echo 0 || echo 1)"

result=$(git ls-files -z '*.md' | xargs -0 -r echo 2>/dev/null) || true
# The temp repo has no .md files; ls-files with no matches should exit 0 with empty output.
assert_pass "git-ls-md-empty" "git ls-files -z *.md empty result is safe" \
    "$([ -z "$result" ] && echo 0 || echo 1)"

# git ls-files with pathspec globs only matches tracked paths at repo root;
# our fixture is nested under dir with spaces/. Verify instead that a
# direct glob at the right level works with NUL.
result=$(git ls-files -z '*.sh' '*.Dockerfile' | xargs -0 -r echo 2>/dev/null) || true
assert_pass "git-ls-multiple-globs" "git ls-files -z with multiple globs survives spaces" \
    "$([ -n "$result" ] && echo 0 || echo 1)"

# ── Test: xargs -0 -r with empty input is harmless ──────────────────────────
result=$(git ls-files -z '*.nonexistent' | xargs -0 -r echo 2>/dev/null || true)
assert_pass "xargs-empty" "xargs -0 -r with empty input is safe" \
    "$([ -z "$result" ] && echo 0 || echo 1)"

# ── Test: filenames with single quotes are handled ──────────────────────────
cat >"it's a test.sh" <<'SHEOF'
#!/bin/sh
echo "test"
SHEOF
chmod +x "it's a test.sh"
git add -A
git commit -q -m "quote fixture"

result=$(git ls-files -z '*.sh' | xargs -0 -r echo 2>/dev/null) || true
assert_pass "git-ls-quote" "git ls-files -z handles single-quote filenames" \
    "$(echo "$result" | grep -q "it's" && echo 0 || echo 1)"

# ── Test: filenames with newlines would be pathological but NUL protects ────
# We test that the xargs -0 delimiter works by checking a space vs NUL scenario.
touch "a b.sh"
chmod +x "a b.sh"
git add "a b.sh"
git commit -q -m "space fixture"

result=$(git ls-files -z '*.sh' | xargs -0 -r printf '%s\n' 2>/dev/null) || true
count=$(echo "$result" | { grep -c '\.sh$' || true; })
# We have: dir with spaces/simple script.sh, it's a test.sh, a b.sh = 3
if [ "$count" -ge 3 ]; then
    echo "PASS: nul-count - NUL-delimited listing found all $count .sh files"
    PASS=$((PASS + 1))
else
    echo "FAIL: nul-count - expected at least 3 .sh files, found $count"
    FAIL=$((FAIL + 1))
fi

echo "nul-safety results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
