#!/bin/sh
# shellcheck disable=SC2016
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
FORCED="$ROOT/deployment/scripts/hosted/forced-command.sh"
DEPLOY="$ROOT/deployment/scripts/hosted/deploy.sh"

assert_contains() {
    grep -Fq -e "$2" "$1" || {
        printf 'Keycloak image contract failed: %s does not contain: %s\n' "$1" "$2" >&2
        exit 1
    }
}

assert_contains "$FORCED" '4) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$FORCED" '5) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$DEPLOY" 'validate_image_reference "$keycloak_image" keycloak'
assert_contains "$DEPLOY" 'verify_image_signature "$keycloak_image"'
assert_contains "$DEPLOY" 'GEOGUESSME_KEYCLOAK_IMAGE=$keycloak_image'
assert_contains "$DEPLOY" 'GEOGUESSME_KEYCLOAK_IMAGE=$previous_keycloak_image'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'docker pull "$KEYCLOAK_IMAGE"'
assert_contains "$ROOT/.github/workflows/release.yml" 'KEYCLOAK_SOURCE: ${{ steps.names.outputs.keycloak_source }}@${{ steps.digests.outputs.keycloak }}'
assert_contains "$ROOT/.github/workflows/release.yml" '[[ "$actual_keycloak" == "$KEYCLOAK_DIGEST" ]]'
assert_contains "$ROOT/.github/workflows/release.yml" '"deploy $BACKEND $WEB $KEYCLOAK $GITHUB_SHA"'
assert_contains "$ROOT/tools/make/deployment.mk" 'images="$$images $${KEYCLOAK_IMAGE}"'
assert_contains "$ROOT/deployment/docker/keycloak-patched/Dockerfile" '2.3.35'
assert_contains "$ROOT/deployment/docker/keycloak-patched/Dockerfile" '0fac87dddd78f1223139e8ef88e819c7f483c0a3835cdf5982ad5e4576d1d896'

assert_forced_command_arity() {
    workflow=$1 expected_count=$2
    command_string=$(sed -n 's/.*"\(deploy \$BACKEND \$WEB.*\$GITHUB_SHA\)".*/\1/p' "$workflow")
    [ -n "$command_string" ] || {
        printf 'Keycloak image contract failed: %s must send the five-field deploy command\n' "$workflow" >&2
        exit 1
    }
    # shellcheck disable=SC2086
    set -- $command_string
    [ "$#" -eq "$expected_count" ] || {
        printf 'Keycloak image contract failed: %s sends %s fields, expected %s\n' \
            "$workflow" "$#" "$expected_count" >&2
        exit 1
    }
}

assert_forced_command_arity "$ROOT/.github/workflows/deploy.yml" 4
assert_forced_command_arity "$ROOT/.github/workflows/release.yml" 5
printf 'Keycloak image contracts passed\n'
