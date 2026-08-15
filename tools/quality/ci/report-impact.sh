#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -gt 2 ]; then
    echo "usage: report-impact.sh [base-ref] [head-ref]" >&2
    exit 2
fi

base_ref=${1:-origin/dev}
head_ref=${2:-HEAD}
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

git rev-parse --verify "${base_ref}^{commit}" >/dev/null 2>&1 || {
    echo "impact: base ref is not a commit: $base_ref" >&2
    exit 2
}
git rev-parse --verify "${head_ref}^{commit}" >/dev/null 2>&1 || {
    echo "impact: head ref is not a commit: $head_ref" >&2
    exit 2
}

changed=$(git diff --name-only "$base_ref" "$head_ref")
classification=$(printf '%s' "$changed" | "$script_dir/classify-changes.sh")

printf 'Change impact: %s..%s\n' "$base_ref" "$head_ref"
printf 'Changed paths: %s\n' "$(printf '%s\n' "$changed" | sed '/^$/d' | wc -l | tr -d ' ')"
printf '%s\n' "$classification"
