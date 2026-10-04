#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../../.." && pwd)
# Aggregate the public Makefile and its responsibility fragments so target
# recipes remain findable.
makefile_agg=$(mktemp /tmp/geoguessme-makefile-aggregate.XXXXXX)
cache_fixture=''
cleanup() {
    if [[ "$makefile_agg" == /tmp/geoguessme-makefile-aggregate.* &&
        "$(realpath "$makefile_agg")" == "$makefile_agg" ]]; then
        rm -f -- "$makefile_agg"
    fi
    if [[ "$cache_fixture" == /tmp/geoguessme-vite-cache.* && -d "$cache_fixture" &&
        ! -L "$cache_fixture" && "$(realpath "$cache_fixture")" == "$cache_fixture" ]]; then
        rm -rf -- "$cache_fixture"
    fi
}
trap cleanup EXIT
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

cache_path=/workspace/frontend/node_modules/.vite-temp
cache_recipe=$(awk '
    /^prepare-frontend-cache:/ { in_target = 1; next }
    in_target && /^[^[:space:]]/ { exit }
    in_target { print }
' "$makefile")
if grep -Fq -- 'node-tools' <<<"$cache_recipe" &&
    grep -Fq -- "-e HOST_UID=\$(TOOLS_UID) -e HOST_GID=\$(TOOLS_GID)" <<<"$cache_recipe" &&
    grep -Fq -- 'test ! -L /workspace/frontend/node_modules/.vite-temp' <<<"$cache_recipe" &&
    grep -Fq -- "chown -R \"\$\$HOST_UID:\$\$HOST_GID\" /workspace/frontend/node_modules/.vite-temp" <<<"$cache_recipe"; then
    pass "frontend cache preparation is Dockerized, narrowly scoped and rejects directory symlinks"
else
    fail "frontend cache preparation lacks Docker, invoking-user ownership or the symlink guard"
fi

for target in build-frontend mobile-init mobile-sync; do
    if grep -Eq "^$target:.*[[:space:]]prepare-frontend-cache([[:space:]]|$)" "$makefile"; then
        pass "$target depends on shared frontend cache preparation"
    else
        fail "$target can build without preparing the frontend cache"
    fi
    recipe=$(awk -v target="$target" '
        $0 ~ "^" target ":" { in_target = 1; next }
        in_target && /^[^[:space:]]/ { exit }
        in_target { print }
    ' "$makefile")
    if grep -Fq -- "\$(TOOLS_USER)" <<<"$recipe" && ! grep -Fq -- "$cache_path" <<<"$recipe"; then
        pass "$target preserves its non-root builder without duplicating cache preparation"
    else
        fail "$target runs its builder as root or duplicates cache ownership logic"
    fi
done

mobile_plan=$(make --no-print-directory -n mobile-sync)
if [[ "$(grep -Fc -- "test ! -L $cache_path" <<<"$mobile_plan")" == 1 ]]; then
    pass "mobile-sync and mobile-init share one cache preparation per Make invocation"
else
    fail "mobile target dependency graph duplicates or skips cache preparation"
fi

# Execute the actual helper script against isolated fixtures, never the shared
# node_modules volume. Extract just the final sh -ec line from its Make recipe.
cache_script=$(sed -n "s/^[[:space:]]*sh -ec '\(.*\)'$/\1/p" <<<"$cache_recipe")
cache_script=${cache_script//\$\$/\$}
if [[ -z "$cache_script" || "$cache_script" != *"$cache_path"* ]]; then
    fail "shared frontend cache helper script cannot be exercised"
else
    cache_fixture=$(mktemp -d /tmp/geoguessme-vite-cache.XXXXXX)
    mkdir -p "$cache_fixture/node_modules/unrelated-package" "$cache_fixture/sentinel"
    modules_owner=$(stat -c '%u:%g' "$cache_fixture/node_modules")
    sibling_owner=$(stat -c '%u:%g' "$cache_fixture/node_modules/unrelated-package")
    sentinel_owner=$(stat -c '%u:%g' "$cache_fixture/sentinel")
    fixture_uid=$(id -u)
    fixture_gid=$(id -g)
    if [[ "$EUID" -eq 0 ]]; then
        fixture_uid=1000
        fixture_gid=1000
    fi
    fixture_cache="$cache_fixture/node_modules/.vite-temp"
    fixture_script=${cache_script//"$cache_path"/"$fixture_cache"}
    HOST_UID="$fixture_uid" HOST_GID="$fixture_gid" sh -ec "$fixture_script"
    if [[ "$(stat -c '%u:%g' "$fixture_cache")" == "$fixture_uid:$fixture_gid" ]]; then
        pass "shared helper creates a missing Vite cache with invoking-user ownership"
    else
        fail "shared helper does not create the Vite cache with correct ownership"
    fi
    mkdir -p "$fixture_cache/nested"
    touch "$fixture_cache/nested/config.mjs"
    HOST_UID="$fixture_uid" HOST_GID="$fixture_gid" sh -ec "$fixture_script"
    if [[ "$(stat -c '%u:%g' "$fixture_cache/nested/config.mjs")" == "$fixture_uid:$fixture_gid" &&
    "$(stat -c '%u:%g' "$cache_fixture/node_modules")" == "$modules_owner" &&
    "$(stat -c '%u:%g' "$cache_fixture/node_modules/unrelated-package")" == "$sibling_owner" ]]; then
        pass "shared helper repairs only cache contents, leaving dependencies and their parent untouched"
    else
        fail "shared helper misses cache contents or changes broader dependency ownership"
    fi
    ln -s "$cache_fixture/sentinel" "$cache_fixture/symlink-cache"
    symlink_script=${cache_script//"$cache_path"/"$cache_fixture/symlink-cache"}
    if HOST_UID="$fixture_uid" HOST_GID="$fixture_gid" sh -ec "$symlink_script"; then
        fail "shared helper accepted a symlink cache directory"
    elif [[ "$(stat -c '%u:%g' "$cache_fixture/sentinel")" == "$sentinel_owner" ]]; then
        pass "shared helper rejects a symlink cache without changing its target"
    else
        fail "shared helper changed the symlink target before rejecting it"
    fi
fi

if [ "$failures" -gt 0 ]; then
    echo "dev-workflow regression FAILED ($failures failure(s))"
    exit 1
fi

echo "dev-workflow regression PASSED"
