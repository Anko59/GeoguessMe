#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$REPO_ROOT"

pass=0
fail=0

ok() {
    echo "PASS: $*"
    pass=$((pass + 1))
}

bad() {
    echo "FAIL: $*"
    fail=$((fail + 1))
}

contains() {
    local file=$1
    local pattern=$2
    local description=$3
    if grep -Eq "$pattern" "$file"; then
        ok "$description"
    else
        bad "$description"
    fi
}

absent() {
    local file=$1
    local pattern=$2
    local description=$3
    if grep -Eq "$pattern" "$file"; then
        bad "$description"
    else
        ok "$description"
    fi
}

CI=.github/workflows/ci.yml
DEPLOY=.github/workflows/deploy.yml
RELEASE=.github/workflows/release.yml
NIGHTLY=.github/workflows/nightly.yml

echo "tiered CI regression tests:"
for workflow in "$CI" "$DEPLOY" "$RELEASE" "$NIGHTLY"; do
    if [ -f "$workflow" ]; then
        ok "$workflow exists"
    else
        bad "$workflow exists"
    fi
done

contains "$CI" '^  pull_request:' "CI runs for pull requests"
absent "$CI" '^  push:' "CI does not duplicate post-merge workflows"
contains "$CI" 'Only the repository dev branch may open a pull request to main' \
    "main accepts release PRs only from dev"
contains "$CI" 'actions/workflows/deploy\.yml/runs' \
    "release PR checks the exact dev deployment"
contains "$CI" 'classify-changes\.sh --null' \
    "changed paths select live-stack suites"
contains "$CI" 'make preflight-docs' "documentation-only PRs use the small gate"
contains "$CI" 'make preflight' "application PRs use the fast gate"
contains "$CI" 'make pr-backend' "backend changes select integration tests"
contains "$CI" 'make pr-frontend' "frontend changes select Chromium E2E"
contains "$CI" 'name: Dockerized verification gate' \
    "stable aggregate status preserves branch protection"
absent "$CI" 'make verify' "pull requests do not run the operational release gate"

contains "$CI" 'retention-days: 7' "failure artifacts have bounded retention"
contains "$CI" 'actions/cache@' "live-stack jobs use persistent Docker caches"
contains "$CI" 'BUILDX_BUILDER:' "Buildx v4 selects its named builder explicitly"
contains "$CI" 'gckeepstorage = 12000000000' "PR BuildKit cache is bounded"

secret_paths=false
while IFS= read -r path; do
    case "$path" in
        *.env | *.env.* | *secret* | *credential* | *token* | *password* | *key*)
            bad "potential secret artifact path: $path"
            secret_paths=true
            ;;
    esac
done < <(sed -n '/^[[:space:]]*path: |$/,/^[[:space:]]*[a-zA-Z_-]*:/p' "$CI" |
    sed -n 's/^[[:space:]]*\([^[:space:]].*\)$/\1/p' || true)
if [ "$secret_paths" = false ]; then
    ok "artifact paths exclude secrets and credentials"
fi

deploy_verify_count=$(grep -c 'make verify' "$DEPLOY" || true)
if [ "$deploy_verify_count" -eq 1 ]; then
    ok "dev performs exactly one complete gate"
else
    bad "dev must perform exactly one complete gate (found $deploy_verify_count)"
fi
contains "$DEPLOY" 'needs: verify' "dev publishing waits for the complete gate"

absent "$RELEASE" 'make verify' "production does not repeat the dev gate"
absent "$RELEASE" 'docker/build-push-action@' "production does not rebuild tested images"
contains "$RELEASE" 'main_tree=\$\(git rev-parse' "release resolves the main tree"
contains "$RELEASE" 'dev_tree=\$\(git rev-parse' "release resolves the tested dev tree"
contains "$RELEASE" 'cosign verify' "release verifies development signatures"
contains "$RELEASE" 'imagetools create' "release promotes immutable manifests"
contains "$RELEASE" 'cosign sign' "release adds the production workflow signature"
contains "$RELEASE" 'actual_backend.*BACKEND_DIGEST' \
    "promotion verifies the backend digest did not change"

contains "$NIGHTLY" 'make verify' "nightly runs the complete operational gate"
contains "$NIGHTLY" 'retention-days: 7' "nightly failure artifacts are bounded"
contains Makefile '^pre-push:.*fast deterministic gate' \
    "pre-push is documented as the fast gate"
contains Makefile '^[[:space:]]+\$\(MAKE\) preflight$' "pre-push invokes preflight"
contains Makefile 'DOCKER_BUILD_FLAGS' "Make supports CI Docker caching"

echo
if [ "$fail" -eq 0 ]; then
    echo "tiered CI regression tests PASSED ($pass passed)"
else
    echo "tiered CI regression tests FAILED ($pass passed, $fail failed)"
    exit 1
fi
