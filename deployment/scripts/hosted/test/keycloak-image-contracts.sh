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
assert_contains "$FORCED" 'production:6) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$FORCED" 'dev:5) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$DEPLOY" 'dev:4)'
assert_contains "$DEPLOY" 'production:5)'
assert_contains "$DEPLOY" 'sops_image=$SOPS_BOOTSTRAP_IMAGE'
assert_contains "$DEPLOY" 'if [ "$sops_image" != "$SOPS_BOOTSTRAP_IMAGE" ]; then'
assert_contains "$DEPLOY" 'validate_image_reference "$keycloak_image" keycloak'
assert_contains "$DEPLOY" 'verify_image_signature "$keycloak_image"'
assert_contains "$DEPLOY" 'GEOGUESSME_KEYCLOAK_IMAGE=$keycloak_image'
assert_contains "$DEPLOY" 'GEOGUESSME_KEYCLOAK_IMAGE=$previous_keycloak_image'
assert_contains "$DEPLOY" 'verify_image_signature "$postgres_image"'
assert_contains "$DEPLOY" 'verify_image_signature "$restic_image"'
assert_contains "$FORCED" 'dev:7) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$FORCED" 'production:8) exec /opt/geoguessme/bin/deploy.sh'
assert_contains "$ROOT/.github/workflows/release.yml" 'KEYCLOAK_SOURCE: ${{ steps.names.outputs.keycloak_source }}@${{ steps.digests.outputs.keycloak }}'
assert_contains "$ROOT/.github/workflows/release.yml" 'run: tools/quality/ci/promote-application-images.sh'
assert_contains "$ROOT/tools/quality/ci/promote-application-images.sh" 'docker buildx imagetools create --tag "$release" "$source"'
assert_contains "$ROOT/tools/quality/ci/promote-application-images.sh" '[[ "$actual_digest" == "$expected_digest" ]]'
assert_contains "$ROOT/.github/workflows/release.yml" '"deploy $BACKEND $WEB $KEYCLOAK $SOPS $POSTGRES $RESTIC $GITHUB_SHA"'
assert_contains "$ROOT/deployment/images/dependencies.tsv" 'deployment/docker/sops-tools/Dockerfile'
assert_contains "$ROOT/deployment/images/dependencies.tsv" 'deployment/docker/postgres-openssl.Dockerfile'
assert_contains "$ROOT/deployment/images/dependencies.tsv" 'deployment/docker/restic-tools.Dockerfile'
assert_contains "$ROOT/deployment/docker/sops-tools/Dockerfile" 'libexpat1=2.5.0-1+deb12u4'
assert_contains "$ROOT/deployment/docker/sops-tools/Dockerfile" 'org.opencontainers.image.source="https://github.com/Anko59/GeoguessMe"'
assert_contains "$DEPLOY" 'validate_sops_image_reference "$sops_image"'
assert_contains "$DEPLOY" 'verify_image_signature "$sops_image"'
assert_contains "$DEPLOY" 'docker pull "$sops_image"'
assert_contains "$DEPLOY" 'SOPS_IMAGE=%s'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'tools/quality/dependency-images/adopt.sh'
assert_contains "$ROOT/tools/quality/dependency-images/adopt.sh" '[[ "$adopted" == "$digest" ]]'
assert_contains "$ROOT/tools/quality/dependency-images/adopt.sh" 'cosign sign --yes -a "revision=$revision"'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'Verify SOPS package is anonymously pullable'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'tagged_image=${ref%@*}'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'repository=${tagged_image%:*}'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'digest=${ref#*@}'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'manifests/$digest'
sops_ref='ghcr.io/anko59/geoguessme-sops:dev-1111111111111111111111111111111111111111@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
ref=${sops_ref#ghcr.io/}
tagged_image=${ref%@*}
repository=${tagged_image%:*}
digest=${ref#*@}
[ "$repository" = 'anko59/geoguessme-sops' ] || exit 1
[ "$digest" = 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' ] || exit 1
assert_contains "$ROOT/.github/workflows/release.yml" 'tools/quality/ci/promote-sops-image.sh "$DEV_SHA"'
if [ ! -x "$ROOT/tools/quality/ci/promote-sops-image.sh" ]; then
    printf 'Image deployment contract failed: SOPS promotion helper must be executable\n' >&2
    exit 1
fi
assert_contains "$ROOT/.github/workflows/release.yml" 'SOPS: ${{ steps.sops-promotion.outputs.image }}@${{ steps.sops-promotion.outputs.digest }}'
assert_contains "$ROOT/tools/quality/ci/promote-sops-image.sh" 'cosign verify'
assert_contains "$ROOT/tools/quality/ci/promote-sops-image.sh" 'docker buildx imagetools create --tag "$release_tag" "$source_image"'
assert_contains "$ROOT/tools/quality/ci/promote-sops-image.sh" '[[ "$promoted_digest" == "$digest" ]]'
assert_contains "$ROOT/.github/workflows/release.yml" 'echo "sops=${{ steps.sops-promotion.outputs.image }}@${{ steps.sops-promotion.outputs.digest }}"'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'socket_proxy: ${{ steps.refs.outputs.socket_proxy }}'
assert_contains "$ROOT/deployment/images/dependencies.tsv" 'deployment/docker/socket-proxy-tools/Dockerfile'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'Sign application images after the exact-digest audit'
assert_contains "$ROOT/.github/workflows/release.yml" 'tools/quality/ci/promote-socket-proxy-image.sh "$DEV_SHA"'
assert_contains "$ROOT/.github/workflows/release.yml" 'SOCKET_PROXY: ${{ steps.sops-promotion.outputs.socket_proxy_image }}@${{ steps.sops-promotion.outputs.socket_proxy_digest }}'
assert_contains "$ROOT/.github/workflows/release.yml" 'socket_proxy=${{ steps.sops-promotion.outputs.socket_proxy_image }}@${{ steps.sops-promotion.outputs.socket_proxy_digest }}'
assert_contains "$ROOT/tools/quality/ci/promote-socket-proxy-image.sh" 'cosign verify'
assert_contains "$ROOT/tools/quality/ci/promote-socket-proxy-image.sh" 'docker buildx imagetools create --tag "$release_tag" "$source_image"'
assert_contains "$ROOT/tools/quality/ci/promote-socket-proxy-image.sh" '[[ "$promoted_digest" == "$digest" ]]'
assert_contains "$ROOT/deployment/docker/keycloak-patched/Dockerfile" 'FROM quay.io/keycloak/keycloak:26.7.5@sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85'
assert_contains "$ROOT/deployment/docker/keycloak-patched/Dockerfile" 'org.opencontainers.image.base.digest="sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85"'
assert_contains "$ROOT/deployment/compose.identity.yaml" 'quay.io/keycloak/keycloak:26.7.5@sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85'
assert_contains "$ROOT/tools/quality/image-scan-exceptions-keycloak.yaml" 'image: quay.io/keycloak/keycloak:26.7.5@sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85'

assert_forced_command_arity() {
    workflow=$1 expected_count=$2
    command_string=$(sed -n 's/.*"\(deploy \$BACKEND \$WEB.*\$GITHUB_SHA\)".*/\1/p' "$workflow")
    [ -n "$command_string" ] || {
        printf 'Image deployment contract failed: %s must send a supported deploy command\n' "$workflow" >&2
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

assert_forced_command_arity "$ROOT/.github/workflows/deploy.yml" 7
assert_forced_command_arity "$ROOT/.github/workflows/release.yml" 8
sops_verify_line=$(grep -nF 'verify_image_signature "$sops_image"' "$DEPLOY" | cut -d: -f1)
sops_pull_line=$(grep -nF 'docker pull "$sops_image"' "$DEPLOY" | cut -d: -f1)
sops_decrypt_line=$(grep -nF '"$sops_image" decrypt' "$DEPLOY" | head -1 | cut -d: -f1)
[ "$sops_verify_line" -lt "$sops_pull_line" ] || {
    printf 'Image deployment contract failed: verify SOPS signature before pulling it\n' >&2
    exit 1
}
[ "$sops_pull_line" -lt "$sops_decrypt_line" ] || {
    printf 'Image deployment contract failed: pull SOPS before decrypting secrets\n' >&2
    exit 1
}
promotion_test_dir=$(mktemp -d /tmp/sops-promotion.XXXXXX)
cleanup_promotion_test() {
    case "$promotion_test_dir" in
        /tmp/sops-promotion.*) rm -rf -- "$promotion_test_dir" ;;
        *)
            printf 'Refusing to remove unexpected test path: %s\n' "$promotion_test_dir" >&2
            exit 1
            ;;
    esac
}
trap cleanup_promotion_test EXIT
mkdir "$promotion_test_dir/bin"
cat >"$promotion_test_dir/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"$TRACE"
if [[ "$3" == inspect ]]; then
    case "$4" in
        *:dev-*) printf '"%s"\n' "$SOURCE_DIGEST" ;;
        *) printf '"%s"\n' "$PROMOTED_DIGEST" ;;
    esac
fi
DOCKER
cat >"$promotion_test_dir/bin/cosign" <<'COSIGN'
#!/usr/bin/env bash
set -euo pipefail
printf 'cosign %s\n' "$*" >>"$TRACE"
COSIGN
chmod +x "$promotion_test_dir/bin/docker" "$promotion_test_dir/bin/cosign"
dev_sha=1111111111111111111111111111111111111111
release_sha=2222222222222222222222222222222222222222
valid_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/trace" \
    SOURCE_DIGEST="$valid_digest" PROMOTED_DIGEST="$valid_digest" \
    GITHUB_SHA="$release_sha" GITHUB_REPOSITORY_OWNER=Anko59 GITHUB_OUTPUT="$promotion_test_dir/success-output" \
    bash "$ROOT/tools/quality/ci/promote-sops-image.sh" "$dev_sha"
grep -Fq "image=ghcr.io/anko59/geoguessme-sops:release-$release_sha" "$promotion_test_dir/success-output"
grep -Fq "digest=$valid_digest" "$promotion_test_dir/success-output"
cosign_line=$(grep -n '^cosign verify' "$promotion_test_dir/trace" | cut -d: -f1)
promote_line=$(grep -n 'docker buildx imagetools create' "$promotion_test_dir/trace" | cut -d: -f1)
[ "$cosign_line" -lt "$promote_line" ] || {
    printf 'SOPS promotion contract failed: signature verification must precede promotion\n' >&2
    exit 1
}
if PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/mismatch-trace" \
    SOURCE_DIGEST="$valid_digest" \
    PROMOTED_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
    GITHUB_SHA="$release_sha" GITHUB_REPOSITORY_OWNER=Anko59 GITHUB_OUTPUT="$promotion_test_dir/mismatch-output" \
    bash "$ROOT/tools/quality/ci/promote-sops-image.sh" "$dev_sha" 2>/dev/null; then
    printf 'SOPS promotion contract failed: digest mismatch must reject promotion\n' >&2
    exit 1
fi
[ ! -s "$promotion_test_dir/mismatch-output" ] || {
    printf 'SOPS promotion contract failed: mismatched digest was exported\n' >&2
    exit 1
}
PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/socket-trace" \
    SOURCE_DIGEST="$valid_digest" PROMOTED_DIGEST="$valid_digest" \
    GITHUB_SHA="$release_sha" GITHUB_REPOSITORY_OWNER=Anko59 GITHUB_OUTPUT="$promotion_test_dir/socket-output" \
    bash "$ROOT/tools/quality/ci/promote-socket-proxy-image.sh" "$dev_sha"
grep -Fq "socket_proxy_image=ghcr.io/anko59/geoguessme-socket-proxy:release-$release_sha" \
    "$promotion_test_dir/socket-output"
grep -Fq "socket_proxy_digest=$valid_digest" "$promotion_test_dir/socket-output"
socket_cosign_line=$(grep -n '^cosign verify' "$promotion_test_dir/socket-trace" | cut -d: -f1)
socket_promote_line=$(grep -n 'docker buildx imagetools create' "$promotion_test_dir/socket-trace" | cut -d: -f1)
[ "$socket_cosign_line" -lt "$socket_promote_line" ] || {
    printf 'Socket-proxy promotion contract failed: signature verification must precede promotion\n' >&2
    exit 1
}
if PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/socket-mismatch-trace" \
    SOURCE_DIGEST="$valid_digest" \
    PROMOTED_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
    GITHUB_SHA="$release_sha" GITHUB_REPOSITORY_OWNER=Anko59 GITHUB_OUTPUT="$promotion_test_dir/socket-mismatch-output" \
    bash "$ROOT/tools/quality/ci/promote-socket-proxy-image.sh" "$dev_sha" 2>/dev/null; then
    printf 'Socket-proxy promotion contract failed: digest mismatch must reject promotion\n' >&2
    exit 1
fi
[ ! -s "$promotion_test_dir/socket-mismatch-output" ] || {
    printf 'Socket-proxy promotion contract failed: mismatched digest was exported\n' >&2
    exit 1
}
PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/app-trace" \
    PROMOTED_DIGEST="$valid_digest" \
    BACKEND_SOURCE="ghcr.io/anko59/geoguessme-backend:dev-$dev_sha@$valid_digest" \
    WEB_SOURCE="ghcr.io/anko59/geoguessme-web:dev-$dev_sha@$valid_digest" \
    KEYCLOAK_SOURCE="ghcr.io/anko59/geoguessme-keycloak:dev-$dev_sha@$valid_digest" \
    BACKEND_RELEASE=ghcr.io/anko59/geoguessme-backend:v1.2.3 \
    WEB_RELEASE=ghcr.io/anko59/geoguessme-web:v1.2.3 \
    KEYCLOAK_RELEASE=ghcr.io/anko59/geoguessme-keycloak:v1.2.3 \
    BACKEND_DIGEST="$valid_digest" WEB_DIGEST="$valid_digest" KEYCLOAK_DIGEST="$valid_digest" \
    bash "$ROOT/tools/quality/ci/promote-application-images.sh"
created_count=$(grep -c 'docker buildx imagetools create' "$promotion_test_dir/app-trace" || true)
[ "$created_count" -eq 3 ] || {
    printf 'Application promotion contract failed: expected three exact manifest retags\n' >&2
    exit 1
}
if PATH="$promotion_test_dir/bin:$PATH" TRACE="$promotion_test_dir/app-mismatch-trace" \
    PROMOTED_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
    BACKEND_SOURCE="ghcr.io/anko59/geoguessme-backend:dev-$dev_sha@$valid_digest" \
    WEB_SOURCE="ghcr.io/anko59/geoguessme-web:dev-$dev_sha@$valid_digest" \
    KEYCLOAK_SOURCE="ghcr.io/anko59/geoguessme-keycloak:dev-$dev_sha@$valid_digest" \
    BACKEND_RELEASE=ghcr.io/anko59/geoguessme-backend:v1.2.3 \
    WEB_RELEASE=ghcr.io/anko59/geoguessme-web:v1.2.3 \
    KEYCLOAK_RELEASE=ghcr.io/anko59/geoguessme-keycloak:v1.2.3 \
    BACKEND_DIGEST="$valid_digest" WEB_DIGEST="$valid_digest" KEYCLOAK_DIGEST="$valid_digest" \
    bash "$ROOT/tools/quality/ci/promote-application-images.sh" 2>/dev/null; then
    printf 'Application promotion contract failed: a retagged digest mismatch must fail\n' >&2
    exit 1
fi
printf 'Hosted image promotion contracts passed\n'
