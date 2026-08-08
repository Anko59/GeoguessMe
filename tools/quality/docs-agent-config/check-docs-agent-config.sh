#!/usr/bin/env bash
# Deterministic documentation and agent-config integrity checker.
#
# Enforced checks:
#   1. Every internal Markdown link in a tracked document resolves to an
#      existing path. URLs, fragment-only links, protocol-relative links, and
#      application-root (leading slash) paths are excluded.
#   2. Backtick repository-path references in agent-config files (the
#      engineering guide, the AGENTS.md files, and the skills) resolve to real
#      files or directories.
#   3. Skills under .agents/skills/ carry the required frontmatter and
#      sections.
#   4. Every canonical document under docs/ is reachable from docs/index.md.
#   5. Runbooks under docs/runbooks/ declare status: archival or status:
#      primary; archival runbooks are never linked from agent instructions.
set -euo pipefail

REPO_ROOT="${DOCS_AGENT_ROOT:-$(git rev-parse --show-toplevel)}"
cd "$REPO_ROOT"

violations_file=$(mktemp)
trap 'rm -f "$violations_file"' EXIT

# Literal sequences that must match paths verbatim (shellcheck-friendly).
DOLLAR_PAREN="\$("
BTP='`'

violation() {
    printf 'violation: %s\n' "$1" >>"$violations_file"
}

# is_external_target: URLs, fragment-only, protocol-relative, and
# application-root (leading slash) paths are not repository references.
is_external_target() {
    case "$1" in
        '' | '#'*) return 0 ;;
        *://* | '//'* | '/'*) return 0 ;;
    esac
    return 1
}

# normalize_path: collapse . and .. components without touching the
# filesystem; __above__ marks an escape above the repository root.
normalize_path() {
    local path=$1
    local -a parts=()
    local -a out=()
    local part
    local escaped=0
    IFS='/' read -r -a parts <<<"$path"
    for part in "${parts[@]}"; do
        case "$part" in
            '' | '.') ;;
            '..')
                if [ "$escaped" -eq 0 ] && [ "${#out[@]}" -gt 0 ]; then
                    unset "out[$((${#out[@]} - 1))]"
                else
                    escaped=1
                fi
                ;;
            *)
                if [ "$escaped" -eq 0 ]; then
                    out+=("$part")
                fi
                ;;
        esac
    done
    if [ "$escaped" -eq 1 ]; then
        printf '__above__'
        return
    fi
    local IFS='/'
    printf '%s' "${out[*]}"
}

check_markdown_links() {
    local file target base_dir resolved
    local -a links=()
    while IFS= read -r -d '' file; do
        mapfile -t links < <(grep -oE '\[[^]]*\]\([^)]*\)' "$file" | sed -nE 's/^.*\]\(([^)]*)\).*/\1/p' || true)
        for target in "${links[@]}"; do
            target=${target//[[:space:]]/}
            target=${target//\"/}
            target=${target//\'/}
            is_external_target "$target" && continue
            target=${target%%#*}
            [ -n "$target" ] || continue
            case "$target" in
                *'*'* | *"$DOLLAR_PAREN"*) continue ;;
            esac
            base_dir=$(dirname "$file")
            resolved=$(normalize_path "$base_dir/$target")
            case "$resolved" in
                __above__)
                    violation "link in $file escapes the repository root: $target"
                    ;;
                */node_modules/* | */dist/* | */coverage/*) ;;
                *)
                    [ -e "$resolved" ] || violation "broken link in $file: $target"
                    ;;
            esac
        done
    done < <(git ls-files -z '*.md')
}

AGENT_FILES=(
    docs/agent-engineering.md
    AGENTS.md
    backend/AGENTS.md
    frontend/AGENTS.md
    deployment/AGENTS.md
)

check_path_references() {
    local file token tok
    if [ -d .agents/skills ]; then
        for file in .agents/skills/*/SKILL.md; do
            [ -f "$file" ] && AGENT_FILES+=("$file")
        done
    fi
    for file in "${AGENT_FILES[@]}"; do
        [ -f "$file" ] || continue
        while IFS= read -r token; do
            tok=${token//\`/}
            [ -n "$tok" ] || continue
            tok=${tok%:[0-9]*}
            tok=${tok%[.,;]}
            case "$tok" in
                '' | *'*'* | *"$DOLLAR_PAREN"* | *[[:space:]]*) continue ;;
            esac
            if [[ ! "$tok" =~ ^(backend|frontend|docs|deployment|infra|tools|Makefile|README\.md|AGENTS\.md|CONTRIBUTING\.md|SECURITY\.md|LICENSE|PRIVACY\.md|\.github|\.githooks|\.agents|\.gitignore|\.markdownlint\.json|\.prettierrc|\.prettierignore|\.stylelintrc\.json|\.shellcheckrc|\.sqlfluff|\.golangci\.yml|\.sops\.yaml|\.dockerignore)(/|$) ]]; then
                continue
            fi
            if [[ "$tok" == */ ]]; then
                [ -d "$tok" ] || violation "path reference in $file not found: $tok"
            else
                [ -e "$tok" ] || violation "path reference in $file not found: $tok"
            fi
        done < <(grep -oE "${BTP}[^${BTP}]+${BTP}" "$file" || true)
    done
}

# skill_has_description: accepts an inline description or a prettier-style
# indented block description in YAML frontmatter.
skill_has_description() {
    local fm=$1
    if printf '%s\n' "$fm" | grep -Eq '^description:[[:space:]]+.+'; then
        return 0
    fi
    printf '%s\n' "$fm" | awk '
        /^description:[[:space:]]*$/ { got=1; next }
        got && NF { if ($0 ~ /^[[:space:]]/) ok=1; exit }
        END { exit !(got && ok) }
    '
}

check_skills() {
    local dir name first fm heading
    if [ ! -d .agents/skills ]; then
        violation 'missing .agents/skills directory'
        return
    fi
    for dir in .agents/skills/*/; do
        [ -d "$dir" ] || continue
        name=$(basename "$dir")
        if [ ! -f "$dir/SKILL.md" ]; then
            violation "skill $name: missing SKILL.md"
            continue
        fi
        first=$(sed -n '1p' "$dir/SKILL.md")
        if [ "$first" != '---' ]; then
            violation "skill $name: SKILL.md must start with YAML frontmatter"
        fi
        fm=$(awk 'NR==1{next} /^---$/{exit} {print}' "$dir/SKILL.md")
        printf '%s\n' "$fm" | grep -Eq "^name:[[:space:]]*${name}[[:space:]]*$" ||
            violation "skill $name: frontmatter name must equal the directory name"
        skill_has_description "$fm" ||
            violation "skill $name: frontmatter requires a non-empty description"
        for heading in '## Workflow' '## Inputs' '## Outputs' '## References'; do
            grep -Fq "$heading" "$dir/SKILL.md" ||
                violation "skill $name: missing required section $heading"
        done
    done
}

check_index_reachability() {
    local file base
    if [ ! -f docs/index.md ]; then
        violation 'missing docs/index.md'
        return
    fi
    for file in docs/*.md; do
        base=$(basename "$file")
        [ "$base" = 'index.md' ] && continue
        grep -Eq "\([^)]*/?$base\)" docs/index.md ||
            violation "canonical doc $file is not linked from docs/index.md"
    done
    for file in docs/runbooks/*.md; do
        [ -f "$file" ] || continue
        base=$(basename "$file")
        grep -Eq "\([^)]*/?$base\)" docs/index.md ||
            violation "runbook $file is not linked from docs/index.md"
    done
}

check_runbooks() {
    local file fm status base agent
    for file in docs/runbooks/*.md; do
        [ -f "$file" ] || continue
        fm=$(awk 'NR==1{next} /^---$/{exit} {print}' "$file")
        status=$(printf '%s\n' "$fm" | sed -nE 's/^status:[[:space:]]*([a-z]+)[[:space:]]*$/\1/p')
        case "$status" in
            archival)
                base=$(basename "$file")
                for agent in "${AGENT_FILES[@]}"; do
                    [ -f "$agent" ] || continue
                    if grep -Eq "\([^)]*/$base\)" "$agent"; then
                        violation "archival runbook $base must not be linked from $agent"
                    fi
                done
                ;;
            primary) ;;
            *)
                violation "runbook $file must declare frontmatter status: archival or status: primary"
                ;;
        esac
    done
}

check_markdown_links
check_path_references
check_skills
check_index_reachability
check_runbooks

if [ -s "$violations_file" ]; then
    cat "$violations_file"
    echo 'docs-agent-config-check FAILED'
    exit 1
fi
echo 'docs-agent-config-check PASSED'
