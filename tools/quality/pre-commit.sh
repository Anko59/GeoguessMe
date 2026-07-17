#!/usr/bin/env bash
set -euo pipefail

# Git supplies the staged-file boundary check. The Dockerized checks are
# repository-wide so a staged rename or deletion cannot evade a structural,
# OpenAPI, or tool-configuration gate.
git diff --cached --check

if ! make format-check; then
    echo "Formatting check failed. Run: make format" >&2
    exit 1
fi
make structure-check lint-go lint-frontend lint-css lint-docs lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style
