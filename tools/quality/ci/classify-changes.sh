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
static_checks=false
backend_unit=false
frontend_unit=false
backend_integration=false
browser_e2e=false
operational=false
docs_candidate=true

classify() {
    local path=$1
    [ -n "$path" ] || return
    seen=true
    case "$path" in
        backend/*)
            docs_candidate=false
            static_checks=true
            backend_unit=true
            backend_integration=true
            ;;
        frontend/*)
            docs_candidate=false
            static_checks=true
            frontend_unit=true
            browser_e2e=true
            ;;
        docs/openapi*)
            docs_candidate=false
            static_checks=true
            backend_unit=true
            frontend_unit=true
            backend_integration=true
            browser_e2e=true
            full=true
            ;;
        .github/workflows/* | deployment/* | infra/* | tools/make/* | Makefile | .dockerignore | .sops.yaml)
            docs_candidate=false
            static_checks=true
            backend_unit=true
            frontend_unit=true
            backend_integration=true
            browser_e2e=true
            operational=true
            full=true
            ;;
        tools/quality/run-integration.sh | tools/quality/integration/*)
            docs_candidate=false
            static_checks=true
            backend_integration=true
            ;;
        tools/quality/run-e2e.sh | tools/quality/e2e/*)
            docs_candidate=false
            static_checks=true
            browser_e2e=true
            ;;
        tools/* | .gitignore)
            docs_candidate=false
            static_checks=true
            ;;
        .github/* | AGENTS.md | */AGENTS.md | .agents/* | docs/* | *.md | LICENSE)
            static_checks=true
            ;;
        *)
            # Unknown files fail safe: exercise both application surfaces.
            docs_candidate=false
            static_checks=true
            backend_unit=true
            frontend_unit=true
            backend_integration=true
            browser_e2e=true
            operational=true
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
    docs_candidate=false
    static_checks=true
    backend_unit=true
    frontend_unit=true
    backend_integration=true
    browser_e2e=true
    operational=true
    full=true
fi
if [ "$full" = true ]; then
    backend_integration=true
    browser_e2e=true
fi

# Compatibility outputs consumed by the current workflow. They intentionally
# mean live-stack suites, not merely that a language appears in the diff.
backend=$backend_integration
frontend=$browser_e2e

docs_only=false
if [ "$seen" = true ] && [ "$docs_candidate" = true ]; then
    docs_only=true
fi

printf 'backend=%s\n' "$backend"
printf 'frontend=%s\n' "$frontend"
printf 'full=%s\n' "$full"
printf 'docs_only=%s\n' "$docs_only"
printf 'static_checks=%s\n' "$static_checks"
printf 'backend_unit=%s\n' "$backend_unit"
printf 'frontend_unit=%s\n' "$frontend_unit"
printf 'backend_integration=%s\n' "$backend_integration"
printf 'browser_e2e=%s\n' "$browser_e2e"
printf 'operational=%s\n' "$operational"
