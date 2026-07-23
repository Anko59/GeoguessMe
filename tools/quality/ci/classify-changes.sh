#!/usr/bin/env bash
set -euo pipefail

null_input=false
if [ "${1:-}" = "--null" ]; then
    null_input=true
elif [ "$#" -ne 0 ]; then
    echo "usage: classify-changes.sh [--null]" >&2
    exit 2
fi

seen=false
backend=false
frontend=false
full=false

classify() {
    local path=$1
    [ -n "$path" ] || return
    seen=true
    case "$path" in
        backend/*)
            backend=true
            ;;
        frontend/*)
            frontend=true
            ;;
        docs/openapi* | .github/* | deployment/* | infra/* | tools/* | Makefile | AGENTS.md | .dockerignore | .gitignore | .sops.yaml)
            full=true
            ;;
        docs/* | *.md | LICENSE) ;;
        *)
            # Unknown files fail safe: exercise both application surfaces.
            full=true
            ;;
    esac
}

if [ "$null_input" = true ]; then
    while IFS= read -r -d '' path; do
        classify "$path"
    done
else
    while IFS= read -r path; do
        classify "$path"
    done
fi

if [ "$seen" = false ]; then
    full=true
fi
if [ "$full" = true ]; then
    backend=true
    frontend=true
fi

docs_only=false
if [ "$seen" = true ] && [ "$backend" = false ] && [ "$frontend" = false ] && [ "$full" = false ]; then
    docs_only=true
fi

printf 'backend=%s\n' "$backend"
printf 'frontend=%s\n' "$frontend"
printf 'full=%s\n' "$full"
printf 'docs_only=%s\n' "$docs_only"
