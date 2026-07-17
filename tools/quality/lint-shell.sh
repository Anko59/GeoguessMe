#!/usr/bin/env bash
# Lint shell scripts with shellcheck. Skips gracefully if not installed.
set -euo pipefail
cd "$(dirname "$0")/../.."
if ! command -v shellcheck >/dev/null 2>&1; then
    echo "shellcheck: not installed (skip)"
    exit 0
fi
fail=0
while IFS= read -r file; do
    if ! shellcheck -x "$file"; then fail=1; fi
done < <(git ls-files | grep '\.sh$')
if [ "$fail" -ne 0 ]; then echo "lint-shell FAILED"; exit 1; fi
echo "lint-shell PASSED"
