#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../../.." && pwd)
makefile="$repo_root/Makefile"
compose_file="$repo_root/deployment/compose.dev.yaml"
failures=0

pass() {
    echo "PASS: $*"
}

fail() {
    echo "FAIL: $*"
    failures=$((failures + 1))
}

dev_recipe=$(awk '
    /^dev:/ { in_target = 1; next }
    in_target && /^[^[:space:]]/ { exit }
    in_target { print }
' "$makefile")

if grep -Fq -- 'up -d --build --renew-anon-volumes' <<<"$dev_recipe"; then
    pass "make dev renews anonymous volumes after rebuilding"
else
    fail "make dev can retain stale anonymous dependency volumes"
fi

if grep -Fq -- '"/app/frontend/node_modules"' "$compose_file"; then
    pass "frontend dependencies remain isolated from the host bind mount"
else
    fail "frontend node_modules anonymous volume is missing"
fi

for volume in geoguessme_dev_db geoguessme_dev_minio; do
    if grep -Fq -- "$volume:" "$compose_file"; then
        pass "$volume remains a named persistent application volume"
    else
        fail "$volume is not declared as a named persistent volume"
    fi
done

if [ "$failures" -gt 0 ]; then
    echo "dev-workflow regression FAILED ($failures failure(s))"
    exit 1
fi

echo "dev-workflow regression PASSED"
