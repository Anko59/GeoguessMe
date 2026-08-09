#!/usr/bin/env bash
set -euo pipefail

sizes=$(mktemp)
directories=$(mktemp)
trap 'rm -f "$sizes" "$directories"' EXIT

declare -A direct_counts=()
declare -A generated=()
while IFS= read -r path || [[ -n "$path" ]]; do
    [[ -z $path || $path == \#* ]] && continue
    generated["$path"]=1
done <tools/quality/generated-files.allowlist

tracked=0
while IFS= read -r -d '' file; do
    tracked=$((tracked + 1))
    [[ -v generated["$file"] ]] && continue
    case "$file" in
        *package-lock.json | *go.sum) continue ;;
        *.go | *.ts | *.tsx | *.js | *.jsx | *.css | *.html | *.sh | *.sql | *.yaml | *.yml | *.tf | *.json | *.toml | *.ini | *.conf | *.env | Dockerfile* | Makefile* | Caddyfile)
            directory=${file%/*}
            [[ $directory == "$file" ]] && directory='.'
            direct_counts["$directory"]=$((${direct_counts["$directory"]:-0} + 1))
            printf '%d\t%s\n' "$(wc -l <"$file")" "$file" >>"$sizes"
            ;;
        *.md) printf '%d\t%s\n' "$(wc -l <"$file")" "$file" >>"$sizes" ;;
    esac
done < <(git ls-files -z)

for directory in "${!direct_counts[@]}"; do
    printf '%d\t%s\n' "${direct_counts[$directory]}" "$directory" >>"$directories"
done

owned_markers=$(git grep -nE '(TODO|FIXME|HACK)\((#[0-9]+|https://github\.com/[^ )]+/issues/[0-9]+|docs/[^ )]+)\)' -- \
    '*.go' '*.ts' '*.tsx' '*.js' '*.jsx' '*.css' '*.html' '*.sh' '*.sql' '*.yaml' '*.yml' '*.tf' \
    ':!tools/quality/debt/**' 2>/dev/null || true)
marker_count=0
[[ -z $owned_markers ]] || marker_count=$(wc -l <<<"$owned_markers")

printf '### Maintenance health\n\n'
tick='`'
fence='```'
printf -- '- Revision: %s%s%s\n' "$tick" "$(git rev-parse --short=12 HEAD)" "$tick"
printf -- '- Tracked files: %d\n' "$tracked"
printf -- '- Owned deferred-work markers: %d\n\n' "$marker_count"
printf '#### Largest source and documentation files\n\n%stext\n' "$fence"
sort -nr "$sizes" | sed -n '1,10p'
printf '%s\n\n#### Densest code/configuration directories\n\n%stext\n' "$fence" "$fence"
sort -nr "$directories" | sed -n '1,10p'
printf '%s\n' "$fence"
