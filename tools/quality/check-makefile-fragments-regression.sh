#!/usr/bin/env bash
# Regression tests for the Makefile fragment split (PR 14).
#
# Guards the invariants CI and documentation rely on after the root Makefile
# was split into included fragments:
#   1. Every fragment referenced by the root `include` lines exists.
#   2. Every fragment stays under the 500-line structure limit.
#   3. Public targets are declared exactly once across the makefiles (no
#      duplicate recipes that silently shadow each other).
#   4. The set of documented targets (## descriptions) in the makefiles exactly
#      matches the set printed by `make help`.
#   5. Every documented target resolves in the make database (`make -pn`).
#   6. Every documented target is declared .PHONY in the root Makefile.
#   7. Aggregate gate dependency ordering is intact (preflight/quality/verify
#      include their canonical components and the PR-14 checks are wired in).
set -u

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT" || exit 1

PASS=0
FAIL=0
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

pass() {
    echo "PASS: $1"
    PASS=$((PASS + 1))
}
fail() {
    echo "FAIL: $1"
    FAIL=$((FAIL + 1))
}

makefiles="Makefile $(sed -nE 's/^include[[:space:]]+//p' Makefile)"

# 1. Every included fragment exists and is included from the root Makefile.
while IFS= read -r frag; do
    [ -n "$frag" ] || continue
    if [ -f "$frag" ]; then
        pass "included fragment exists: $frag"
    else
        fail "included fragment missing: $frag"
    fi
    grep -qE "^include .*$frag" Makefile || fail "fragment $frag is not included from the root Makefile"
done < <(grep -oE '^include .*' Makefile | awk '{for (i = 2; i <= NF; i++) print $i}')

# 2. Every fragment stays under the 500-line structure limit.
for frag in $makefiles; do
    [ -f "$frag" ] || continue
    lines=$(wc -l <"$frag")
    if [ "$lines" -gt 500 ]; then
        fail "fragment $frag exceeds 500 lines ($lines)"
    else
        pass "fragment $frag within 500 lines ($lines)"
    fi
done

# 3. No duplicate public target declarations across the makefiles.
for frag in $makefiles; do
    [ -f "$frag" ] || continue
    sed -nE 's/^([a-zA-Z0-9_.-]+):.*##.*/\1/p' "$frag"
done | sort >"$tmpdir/declared-all.txt"
sort -u "$tmpdir/declared-all.txt" >"$tmpdir/declared-unique.txt"
dup=$(comm -23 "$tmpdir/declared-all.txt" "$tmpdir/declared-unique.txt")
if [ -z "$dup" ]; then
    pass "no duplicate target declarations"
else
    fail "duplicate target declarations: $(echo "$dup" | tr '\n' ' ')"
fi

# 4. Documented targets exactly match `make help` output (ANSI-stripped).
# The help target line is `  make <target>` (usage text); exclude it.
make help 2>/dev/null | sed 's/\x1b\[[0-9;]*m//g' |
    grep -E '^  [a-zA-Z0-9_.-]+ +' | grep -v '^  make ' |
    awk '{print $1}' | sort -u >"$tmpdir/help-targets.txt"
missing_from_help=$(comm -23 "$tmpdir/declared-unique.txt" "$tmpdir/help-targets.txt")
extra_in_help=$(comm -13 "$tmpdir/declared-unique.txt" "$tmpdir/help-targets.txt")
if [ -z "$missing_from_help" ] && [ -z "$extra_in_help" ]; then
    pass "documented targets exactly match make help ($(wc -l <"$tmpdir/help-targets.txt") targets)"
else
    [ -n "$missing_from_help" ] && fail "documented targets missing from make help: $(echo "$missing_from_help" | tr '\n' ' ')"
    [ -n "$extra_in_help" ] && fail "help targets not documented in makefiles: $(echo "$extra_in_help" | tr '\n' ' ')"
fi

# 5. Every documented target resolves in the make database.
make -pn 2>/dev/null | grep -oE '^[a-zA-Z0-9_.-]+:' | tr -d ':' | sort -u >"$tmpdir/db-targets.txt"
unresolved=$(comm -23 "$tmpdir/declared-unique.txt" "$tmpdir/db-targets.txt")
if [ -z "$unresolved" ]; then
    pass "every documented target resolves"
else
    fail "targets do not resolve: $(echo "$unresolved" | tr '\n' ' ')"
fi

# 6. Every documented target is declared .PHONY in the root Makefile.
# The .PHONY declaration spans backslash-continuation lines, so join them
# first, then extract the names after the keyword.
sed -E -e ':a' -e '/\\$/N' -e 's/\\\n/ /' -e 'ta' Makefile |
    grep '^\.PHONY:' |
    sed 's/^\.PHONY:[[:space:]]*//' |
    tr ' \t' '\n' | sed '/^$/d' | sort -u >"$tmpdir/phony.txt"
not_phony=$(comm -23 "$tmpdir/declared-unique.txt" "$tmpdir/phony.txt")
if [ -z "$not_phony" ]; then
    pass "every documented target is declared .PHONY in the root Makefile"
else
    fail "targets not declared .PHONY: $(echo "$not_phony" | tr '\n' ' ')"
fi

# 7. Aggregate gate dependency ordering.
preflight_line=$(grep -E '^preflight:' tools/make/quality.mk || true)
quality_line=$(grep -E '^quality:' tools/make/quality.mk || true)
verify_line=$(grep -E '^verify:' tools/make/quality.mk || true)
for dep in structure-check openapi-check archcheck audit test-unit; do
    printf '%s' "$preflight_line" | grep -qE "(^|[[:space:]])$dep([[:space:]]|$)" || fail "preflight is missing dependency $dep"
done
# The seven harness self-tests are path-triggered in preflight via the
# HARNESS_GATE expansion; quality still runs them unconditionally.
printf '%s' "$preflight_line" | grep -qE '\(HARNESS_GATE\)' || fail "preflight does not select harness self-tests via HARNESS_GATE"
for dep in test-makefile-fragments-regression test-structure-regression test-debt-markers-regression test-docs-agent-config test-ci-classifier test-e2e-regression test-dev-workflow-regression; do
    grep -qE "^HARNESS_GATE_TARGETS.*$dep" tools/make/quality.mk || fail "HARNESS_GATE_TARGETS is missing $dep"
    printf '%s' "$quality_line" | grep -qE "(^|[[:space:]])$dep([[:space:]]|$)" || fail "quality is missing harness dependency $dep"
done
pass "preflight gate ordering intact"
for dep in structure-check openapi-check archcheck test-archcheck-regression test-makefile-fragments-regression audit test-verified; do
    printf '%s' "$quality_line" | grep -qE "(^|[[:space:]])$dep([[:space:]]|$)" || fail "quality is missing dependency $dep"
done
pass "quality gate ordering intact"
for dep in quality test-integration test-e2e container-verify migration-test smoke load-test; do
    printf '%s' "$verify_line" | grep -qE "(^|[[:space:]])$dep([[:space:]]|$)" || fail "verify is missing dependency $dep"
done
pass "verify gate ordering intact"

echo
echo "makefile-fragments-regression: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
