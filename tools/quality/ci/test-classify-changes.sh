#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
CLASSIFIER="$SCRIPT_DIR/classify-changes.sh"

assert_scope() {
    local name=$1
    local input=$2
    local expected=$3
    local actual
    actual=$(printf '%s' "$input" | "$CLASSIFIER")
    if [ "$actual" != "$expected" ]; then
        printf 'FAIL: %s\nexpected:\n%s\nactual:\n%s\n' "$name" "$expected" "$actual" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

assert_scope "documentation only" $'README.md\ndocs/testing.md\n' \
    $'backend=false\nfrontend=false\nfull=false\ndocs_only=true\nstatic_checks=true\nbackend_unit=false\nfrontend_unit=false\nbackend_integration=false\nbrowser_e2e=false\noperational=false\nharness=false'
assert_scope "backend only" $'backend/handlers/auth.go\nbackend/handlers/auth_test.go\n' \
    $'backend=true\nfrontend=false\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=false\nbackend_integration=true\nbrowser_e2e=false\noperational=false\nharness=false'
assert_scope "frontend only" $'frontend/src/App.tsx\nfrontend/src/App.test.tsx\n' \
    $'backend=false\nfrontend=true\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=false\nfrontend_unit=true\nbackend_integration=false\nbrowser_e2e=true\noperational=false\nharness=false'
assert_scope "shared operations" $'.github/workflows/ci.yml\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'
assert_scope "API contract" $'docs/openapi.yaml\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=false\nharness=false'
assert_scope "quality tooling" $'tools/quality/ci/classify-changes.sh\nAGENTS.md\n' \
    $'backend=false\nfrontend=false\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=false\nfrontend_unit=false\nbackend_integration=false\nbrowser_e2e=false\noperational=false\nharness=true'
assert_scope "integration harness" $'tools/quality/run-integration.sh\n' \
    $'backend=true\nfrontend=false\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=false\nfrontend_unit=false\nbackend_integration=true\nbrowser_e2e=false\noperational=false\nharness=true'
assert_scope "makefile change" $'Makefile\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'
assert_scope "make fragment change" $'tools/make/tests.mk\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'
assert_scope "dev compose change" $'deployment/compose.dev.yaml\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'
assert_scope "qa tooling stays non-harness" $'tools/qa/run-local.sh\n' \
    $'backend=false\nfrontend=false\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=false\nfrontend_unit=false\nbackend_integration=false\nbrowser_e2e=false\noperational=false\nharness=false'
assert_scope "unknown path fails safe" $'unexpected.file\n' \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'
assert_scope "empty diff fails safe" "" \
    $'backend=true\nfrontend=true\nfull=true\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=true\nharness=true'

null_actual=$(printf 'backend/main.go\0frontend/src/App.tsx\0' | "$CLASSIFIER" --null)
[ "$null_actual" = $'backend=true\nfrontend=true\nfull=false\ndocs_only=false\nstatic_checks=true\nbackend_unit=true\nfrontend_unit=true\nbackend_integration=true\nbrowser_e2e=true\noperational=false\nharness=false' ] || {
    echo "FAIL: NUL-delimited input" >&2
    exit 1
}
echo "PASS: NUL-delimited input"
echo "CI change classifier tests PASSED"
