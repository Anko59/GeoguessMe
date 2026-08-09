#!/usr/bin/env bash
# Regression tests for the docs/agent-config checker.
#
# Each scenario builds a small fixture repository, commits it, and asserts the
# checker either passes or fails with the expected diagnostic. Fixtures live in
# a temporary directory and never touch the working tree.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CHECKER="$REPO_ROOT/tools/quality/docs-agent-config/check-docs-agent-config.sh"
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "$TMP_ROOT"' EXIT

failures=0

# make_fixture <dir> builds a minimal valid documentation tree.
make_fixture() {
    local dir=$1
    mkdir -p "$dir/docs/runbooks" "$dir/.agents/skills/demo-skill"
    cat >"$dir/docs/index.md" <<'EOF'
# Index

- [Alpha](alpha.md)
- [Beta](beta.md)
- [Engineering guide](agent-engineering.md)
- [Primary runbook](runbooks/primary.md)
- [Archived runbook](runbooks/archived.md)
EOF
    printf '# Alpha\n\nSee [beta](beta.md).\n' >"$dir/docs/alpha.md"
    printf '# Beta\n\nBack to [index](index.md).\n' >"$dir/docs/beta.md"
    cat >"$dir/docs/agent-engineering.md" <<'EOF'
# Engineering guide

Guides reference [the index](index.md) only.
EOF
    cat >"$dir/docs/runbooks/primary.md" <<'EOF'
---
status: primary
---

# Primary runbook
EOF
    cat >"$dir/docs/runbooks/archived.md" <<'EOF'
---
status: archival
---

# Archived runbook
EOF
    cat >"$dir/.agents/skills/demo-skill/SKILL.md" <<'EOF'
---
name: demo-skill
description:
    Demo skill for checker regression tests, using the prettier-style
    indented block description.
---

## Workflow

## Inputs

## Outputs

## References
EOF
}

init_fixture_git() {
    local dir=$1
    git -C "$dir" init -q
    git -C "$dir" config user.name checker-test
    git -C "$dir" config user.email checker-test@example.invalid
    git -C "$dir" add -A
    git -C "$dir" commit -q -m fixtures
}

run_checker() {
    local dir=$1
    (cd "$dir" && DOCS_AGENT_ROOT="$dir" "$CHECKER")
}

assert_pass() {
    local dir=$1 label=$2
    if run_checker "$dir" >"$TMP_ROOT/out" 2>&1; then
        echo "ok: $label"
    else
        echo "FAIL: $label expected a passing checker"
        cat "$TMP_ROOT/out"
        failures=$((failures + 1))
    fi
}

assert_fail_with() {
    local dir=$1 needle=$2 label=$3
    if run_checker "$dir" >"$TMP_ROOT/out" 2>&1; then
        echo "FAIL: $label expected the checker to fail"
        failures=$((failures + 1))
    elif grep -Fq "$needle" "$TMP_ROOT/out"; then
        echo "ok: $label"
    else
        echo "FAIL: $label expected output to contain '$needle'"
        cat "$TMP_ROOT/out"
        failures=$((failures + 1))
    fi
}

scenario_valid_tree() {
    local dir=$TMP_ROOT/valid
    make_fixture "$dir"
    init_fixture_git "$dir"
    assert_pass "$dir" "valid tree passes"
}

scenario_broken_link() {
    local dir=$TMP_ROOT/broken-link
    make_fixture "$dir"
    printf '# Beta\n\nMissing [target](missing.md).\n' >"$dir/docs/beta.md"
    init_fixture_git "$dir"
    assert_fail_with "$dir" "broken link in docs/beta.md: missing.md" "broken link is reported"
}

scenario_broken_reference_link() {
    local dir=$TMP_ROOT/broken-reference-link
    make_fixture "$dir"
    cat >"$dir/docs/beta.md" <<'EOF'
# Beta

Missing [reference target][missing].

[missing]: missing.md
EOF
    init_fixture_git "$dir"
    assert_fail_with "$dir" "broken link in docs/beta.md: missing.md" "broken reference-style link is reported"
}

scenario_undefined_reference_link() {
    local dir=$TMP_ROOT/undefined-reference-link
    make_fixture "$dir"
    printf '# Beta\n\nMissing [reference definition][missing].\n' >"$dir/docs/beta.md"
    init_fixture_git "$dir"
    assert_fail_with "$dir" "undefined reference link in docs/beta.md: missing" "undefined reference-style link is reported"
}

scenario_doc_not_in_index() {
    local dir=$TMP_ROOT/not-indexed
    make_fixture "$dir"
    printf '# Gamma\n' >"$dir/docs/gamma.md"
    init_fixture_git "$dir"
    assert_fail_with "$dir" "canonical doc docs/gamma.md is not linked from docs/index.md" "unindexed doc is reported"
}

scenario_bad_path_reference() {
    local dir=$TMP_ROOT/bad-path
    make_fixture "$dir"
    cat >"$dir/docs/agent-engineering.md" <<'EOF'
# Engineering guide

Guides reference `docs/nonexistent.md`.
EOF
    init_fixture_git "$dir"
    assert_fail_with "$dir" "path reference in docs/agent-engineering.md not found: docs/nonexistent.md" "missing path reference is reported"
}

scenario_missing_skill_frontmatter() {
    local dir=$TMP_ROOT/no-frontmatter
    make_fixture "$dir"
    cat >"$dir/.agents/skills/demo-skill/SKILL.md" <<'EOF'
# Demo skill

## Workflow

## Inputs

## Outputs

## References
EOF
    init_fixture_git "$dir"
    assert_fail_with "$dir" "SKILL.md must start with YAML frontmatter" "missing skill frontmatter is reported"
}

scenario_archival_linked_from_agent() {
    local dir=$TMP_ROOT/archival-linked
    make_fixture "$dir"
    cat >"$dir/docs/agent-engineering.md" <<'EOF'
# Engineering guide

Guides reference [archived](runbooks/archived.md).
EOF
    init_fixture_git "$dir"
    assert_fail_with "$dir" "archival runbook archived.md must not be linked from docs/agent-engineering.md" "archival runbook link is reported"
}

scenario_runbook_without_status() {
    local dir=$TMP_ROOT/no-status
    make_fixture "$dir"
    printf '# Primary runbook\n' >"$dir/docs/runbooks/primary.md"
    init_fixture_git "$dir"
    assert_fail_with "$dir" "runbook docs/runbooks/primary.md must declare frontmatter status" "missing runbook status is reported"
}

scenario_valid_tree
scenario_broken_link
scenario_broken_reference_link
scenario_undefined_reference_link
scenario_doc_not_in_index
scenario_bad_path_reference
scenario_missing_skill_frontmatter
scenario_archival_linked_from_agent
scenario_runbook_without_status

if [ "$failures" -ne 0 ]; then
    echo "docs-agent-config regression tests FAILED: $failures scenario(s)"
    exit 1
fi
echo 'docs-agent-config regression tests PASSED'
