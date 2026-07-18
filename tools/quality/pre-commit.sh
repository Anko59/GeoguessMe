#!/usr/bin/env bash
set -euo pipefail

# Git supplies the staged-file boundary check. The Dockerized checks are
# repository-wide so a staged rename or deletion cannot evade a structural,
# OpenAPI, or tool-configuration gate.
git diff --cached --check

# Require a clean working tree: no unstaged or untracked changes in tracked
# files. This prevents accidental partial commits and ensures the pre-commit
# gate runs against the same content that will be committed.
if ! git diff-files --quiet; then
    echo "Working tree has unstaged changes. Stage or stash them before committing." >&2
    exit 1
fi
if [ -n "$(git ls-files --others --exclude-standard)" ]; then
    echo "Working tree has untracked files. Track, ignore, or stash them before committing." >&2
    exit 1
fi

if ! make format-check; then
    echo "Formatting check failed. Run: make format" >&2
    exit 1
fi
make structure-check test-structure-regression lint-go lint-frontend lint-css lint-docs lint-shell lint-docker lint-actions lint-sql lint-caddy lint-openapi check-e2e-style
