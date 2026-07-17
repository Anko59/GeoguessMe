#!/usr/bin/env bash
# Regression tests for the GeoGuessMe structure checker.
# Creates temporary repos with known violations and asserts the checker catches them.
set -euo pipefail

CHECKER="$(cd "$(dirname "$0")/.." && pwd)/structure-check"
PASS=0; FAIL=0

assert_pass() {
    echo -n "  $1 ... " && "$CHECKER" >/dev/null 2>&1 && echo "PASS" && PASS=$((PASS+1)) || { echo "FAIL (expected pass)"; FAIL=$((FAIL+1)); }
}
assert_fail() {
    echo -n "  $1 ... " && ! "$CHECKER" >/dev/null 2>&1 && echo "PASS" && PASS=$((PASS+1)) || { echo "FAIL (expected failure)"; FAIL=$((FAIL+1)); }
}

with_temp_repo() {
    local dir
    dir=$(mktemp -d)
    trap "rm -rf $dir" RETURN
    cd "$dir"
    git init -q
    git config user.email "test@localhost"
    git config user.name "test"
    git add -A
    git commit -q -m init
    "$@"
    cd - >/dev/null
    rm -rf "$dir"
}

echo "structure-check regression tests:"

# Test 1: clean repo passes
echo "  clean repo ..."
(cd "$(dirname "$CHECKER")/../.." && assert_pass "current repo")

# Test 2: a Go file with 501 lines fails
with_temp_repo bash -c '
  (for i in $(seq 1 501); do echo "// line $i"; done) > big.go
  assert_fail "go file >500 lines"
'

# Test 3: a markdown file with 600 lines still passes (not code)
with_temp_repo bash -c '
  (for i in $(seq 1 600); do echo "line $i"; done) > big.md
  assert_pass "md file >500 lines (exempt)"
'

# Test 4: directory with 15 children fails
with_temp_repo bash -c '
  mkdir toomany
  for i in $(seq 1 15); do echo "export {};" > "toomany/f$i.ts"; done
  assert_fail "15 code children > 14"
'

# Test 5: directory with 14 children passes
with_temp_repo bash -c '
  mkdir justright
  for i in $(seq 1 14); do echo "export {};" > "justright/f$i.ts"; done
  assert_pass "14 code children"
'

# Test 6: PNGS/package-lock/go.sum are exempt
with_temp_repo bash -c '
  mkdir deep
  for i in $(seq 1 20); do echo "/* this is fine */" > "deep/f$i.ts"; done
  # Wait, 20 code children FAIL.
  # Restart with 14.
'
# Actually, 20 code children > 14 — it should fail. Let me test correctly:
with_temp_repo bash -c '
  mkdir deep
  for i in $(seq 1 14); do echo "export {};" > "deep/f$i.ts"; done
  # Add non-code files (exempt from count)
  touch deep/a.png deep/b.jpg
  assert_pass "directory with 14 code + 2 png children"
'

echo ""
echo "results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
