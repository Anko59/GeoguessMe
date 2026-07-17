#!/usr/bin/env bash
set -euo pipefail; cd "$(dirname "$0")/../.."; fail=0
if ! command -v hadolint >/dev/null; then echo "hadolint: skip"; exit 0; fi
for f in deployment/docker/*.Dockerfile; do
    hadolint "$f" || fail=1
done
[ "$fail" -ne 0 ] && { echo "lint-docker FAILED"; exit 1; }
echo "lint-docker PASSED"
