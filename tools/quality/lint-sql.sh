#!/usr/bin/env bash
set -euo pipefail; cd "$(dirname "$0")/../.."; fail=0
if ! command -v sqlfluff >/dev/null; then echo "sqlfluff: skip"; exit 0; fi
for f in backend/internal/database/migrations/*.sql; do
    sqlfluff lint --dialect postgres "$f" || fail=1
done
[ "$fail" -ne 0 ] && { echo "lint-sql FAILED"; exit 1; }
echo "lint-sql PASSED"
