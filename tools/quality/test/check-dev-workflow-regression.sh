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

dev_social_recipe=$(awk '
    /^dev-social:/ { in_target = 1; next }
    in_target && /^[^[:space:]]/ { exit }
    in_target { print }
' "$makefile")

if grep -Fq -- 'GEOGUESSME_DEV_PUBLIC_URL=https://geoguessme.localhost' <<<"$dev_social_recipe" &&
    grep -Fq -- 'local-keycloak local-caddy' <<<"$dev_social_recipe"; then
    pass "social development starts the HTTPS identity entrypoint before the application"
else
    fail "social development does not bootstrap Caddy and the HTTPS public origin"
fi

if grep -Fq -- 'GEOGUESSME_GOOGLE_CLIENT_JSON' <<<"$dev_social_recipe" &&
    grep -Fq -- "jq -er '.web.client_secret'" <<<"$dev_social_recipe"; then
    pass "social development can load ignored Google credentials without printing or committing them"
else
    fail "social development cannot load a Google OAuth client JSON"
fi

if grep -Fq -- '--wait --wait-timeout 180 backend' <<<"$dev_social_recipe" &&
    grep -Fq -- '--no-deps frontend oauth2-proxy' <<<"$dev_social_recipe"; then
    pass "social development waits for backend OIDC discovery before starting browser-facing services"
else
    fail "social development can race backend OIDC discovery against the HTTPS identity entrypoint"
fi

for contract in \
    'local-caddy:' \
    '127.0.0.1:443:443' \
    'auth-dev.geoguessme.com:host-gateway' \
    'OIDC_ISSUER_URL: https://auth-dev.geoguessme.com/realms/geoguessme' \
    'OAUTH2_PROXY_REDIRECT_URL: https://geoguessme.localhost/oauth2/callback'; do
    if grep -Fq -- "$contract" "$compose_file"; then
        pass "local HTTPS contract present: $contract"
    else
        fail "local HTTPS contract missing: $contract"
    fi
done

if grep -Eq -- 'http://(auth\.)?geoguessme\.localhost' "$compose_file"; then
    fail "social development still contains a plain-HTTP GeoGuessMe browser origin"
else
    pass "social development has no plain-HTTP GeoGuessMe browser origin"
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

if grep -Fq -- "\$(COMPOSE_DEV) --profile social down -v --remove-orphans" "$makefile"; then
    pass "development reset includes local Keycloak and its identity volume"
else
    fail "development reset leaves the social-auth identity volume behind"
fi

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
