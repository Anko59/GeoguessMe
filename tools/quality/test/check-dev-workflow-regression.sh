#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../../.." && pwd)
# Aggregate the public Makefile and its responsibility fragments so target
# recipes remain findable.
makefile_agg=$(mktemp)
trap 'rm -f "$makefile_agg"' EXIT
cat "$repo_root"/Makefile "$repo_root"/tools/make/*.mk >"$makefile_agg"
makefile="$makefile_agg"
compose_file="$repo_root/deployment/compose.dev.yaml"
dockerignore="$repo_root/.dockerignore"
deployment_make="$repo_root/tools/make/deployment.mk"
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

if grep -Fq -- 'up -d --build' <<<"$dev_recipe" && ! grep -Fq -- '--renew-anon-volumes' <<<"$dev_recipe"; then
    pass "make dev rebuilds without allocating replacement anonymous volumes"
else
    fail "make dev must rebuild without renewing anonymous volumes"
fi

if grep -Fq -- '"frontend-node-modules:/app/frontend/node_modules"' "$compose_file"; then
    pass "frontend dependencies use one reusable named volume"
else
    fail "frontend node_modules named volume is missing"
fi

if grep -Fq -- 'npm install --prefer-offline --no-audit' "$compose_file"; then
    pass "frontend startup refreshes the reusable dependency volume"
else
    fail "frontend startup does not refresh dependencies after lockfile changes"
fi

for volume in geoguessme_dev_db geoguessme_dev_minio frontend-node-modules; do
    if grep -Fq -- "$volume:" "$compose_file"; then
        pass "$volume remains a named persistent application volume"
    else
        fail "$volume is not declared as a named persistent volume"
    fi
done

if grep -Fxq 'security/image-reports' "$dockerignore"; then
    pass "image-audit artifacts are excluded from Docker build contexts"
else
    fail "security/image-reports is missing from .dockerignore"
fi

if grep -Fq 'trap cleanup_image_archive EXIT' "$deployment_make" &&
    grep -Fq 'cleanup_image_archive()' "$deployment_make"; then
    pass "image-audit archives are cleaned when the audit shell exits"
else
    fail "audit-images lacks failure-safe image archive cleanup"
fi

if [ "$failures" -gt 0 ]; then
    echo "dev-workflow regression FAILED ($failures failure(s))"
    exit 1
fi

echo "dev-workflow regression PASSED"
