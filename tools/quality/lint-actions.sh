#!/usr/bin/env bash
set -euo pipefail; cd "$(dirname "$0")/../.."; fail=0
if ! command -v actionlint >/dev/null; then echo "actionlint: skip"; exit 0; fi
for f in .github/workflows/*.yml; do
    actionlint "$f" || fail=1
done
[ "$fail" -ne 0 ] && { echo "lint-actions FAILED"; exit 1; }
echo "lint-actions PASSED"
