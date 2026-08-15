#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
marker_re='(TODO|FIXME|HACK)'
owned_re='(TODO|FIXME|HACK)\((#[0-9]+|https://github\.com/[^[:space:])]+/issues/[0-9]+|docs/[A-Za-z0-9_./-]+(#[A-Za-z0-9_.-]+)?)\)'
failed=0

while IFS= read -r -d '' file; do
    case "$file" in
        tools/quality/debt/*.sh) continue ;;
        *.go | *.ts | *.tsx | *.js | *.jsx | *.css | *.html | *.sh | *.sql | *.yaml | *.yml | *.tf | Dockerfile*) ;;
        *) continue ;;
    esac

    line_number=0
    while IFS= read -r line || [[ -n "$line" ]]; do
        line_number=$((line_number + 1))
        [[ $line =~ $marker_re ]] || continue
        remaining=$(sed -E "s@$owned_re@@g" <<<"$line")
        if [[ $remaining =~ $marker_re ]]; then
            printf '%s:%d: unowned debt marker; use TODO(#123), TODO(https://github.com/OWNER/REPO/issues/123), or TODO(docs/path.md#entry)\n' \
                "$file" "$line_number" >&2
            failed=1
        fi
    done <"$root/$file"
done < <(git -C "$root" ls-files -z)

exit "$failed"
