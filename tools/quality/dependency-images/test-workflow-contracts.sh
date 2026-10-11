#!/usr/bin/env bash
# Structural CI trust contracts plus deterministic, registry-free promotion tests.
# Canonical runner: make test-security-workflows (Dockerized go-security).
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TMP=$(mktemp -d)
[[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] || exit 1
cleanup() { [[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] && rm -rf -- "$TMP"; }
trap cleanup EXIT
PASS=0
fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}
pass() {
    PASS=$((PASS + 1))
    printf 'PASS: %s\n' "$*"
}
contains() { grep -Fq -- "$2" "$1" || fail "missing contract '$2' in ${1##*/}"; }
absent() { ! grep -Fq -- "$2" "$1" || fail "unexpected contract '$2' in ${1##*/}"; }
first_line() { awk -v needle="$2" 'index($0,needle) {print NR; exit}' "$1"; }
before() {
    local first second
    first=$(first_line "$1" "$2")
    second=$(first_line "$1" "$3")
    [[ -n "$first" && -n "$second" && "$first" -lt "$second" ]] || fail "order: $2 must precede $3"
}
job() {
    awk -v name="$2" '$0=="  "name":" {active=1; print; next} active && /^  [a-zA-Z0-9_-]+:/ {exit} active {print}' "$1"
}
mkdir -p "$TMP/workflows"
for workflow in deploy security nightly release; do cp "$ROOT/.github/workflows/$workflow.yml" "$TMP/workflows/$workflow.yml"; done
DEPLOY="$TMP/workflows/deploy.yml"
SECURITY="$TMP/workflows/security.yml"
NIGHTLY="$TMP/workflows/nightly.yml"
RELEASE="$TMP/workflows/release.yml"
job "$DEPLOY" dependencies >"$TMP/dependencies"
job "$DEPLOY" publish >"$TMP/publish"
job "$SECURITY" audit-images >"$TMP/security-audit"
job "$RELEASE" promote >"$TMP/release-promote"
job "$RELEASE" deploy >"$TMP/release-deploy"
job "$DEPLOY" deploy >"$TMP/dev-deploy"
for file in "$TMP/dev-deploy" "$TMP/release-deploy"; do
    contains "$file" 'vars.HOSTED_DEPENDENCY_PROTOCOL_READY'
    contains "$file" "[ \"\$PROTOCOL_READY\" = true ]"
    before "$file" 'Require verified operator runtime cutover' 'Deploy through Cloudflare Access'
done
pass 'utility-aware deployment requires explicit verified operator cutover'

for file in "$TMP/dependencies" "$TMP/security-audit"; do
    contains "$file" 'group: geoguessme-dependency-publication'
    contains "$file" 'cancel-in-progress: false'
    absent "$file" 'cancel-in-progress: true'
done
contains "$TMP/dependencies" 'make publish-security-images'
contains "$TMP/dependencies" 'packages: write'
contains "$TMP/dependencies" 'id-token: write'
pass 'Security and development serialize dependency publication without cancellation'
for file in "$TMP/dependencies" "$TMP/security-audit" "$TMP/publish" "$TMP/release-promote" "$NIGHTLY"; do
    contains "$file" 'uses: sigstore/cosign-installer@'
    contains "$file" 'cosign-release: v2.6.5'
    contains "$file" 'uses: docker/login-action@'
done
pass 'artifact-consuming workflows install pinned Cosign and authenticated GHCR access'

components=(keycloak restic sops socket_proxy postgres cloudflared caddy_runtime)
for gate in quality integration e2e operational; do
    job "$DEPLOY" "$gate" >"$TMP/gate-$gate"
    block="$TMP/gate-$gate"
    contains "$block" 'needs: dependencies'
    for component in "${components[@]}"; do
        key=$(printf '%s_IMAGE' "$component" | tr '[:lower:]' '[:upper:]')
        contains "$block" "$key: \${{ needs.dependencies.outputs.$component }}"
    done
    before "$block" 'uses: sigstore/cosign-installer@' 'make bootstrap'
    before "$block" 'uses: docker/login-action@' 'make bootstrap'
done
pass 'every runtime-consuming gate receives immutable dependency outputs before preparation'
contains "$TMP/publish" 'needs: [dependencies, quality, integration, e2e, mobile, operational]'
contains "$TMP/publish" 'file: deployment/docker/backend.Dockerfile'
contains "$TMP/publish" 'file: deployment/docker/frontend.Dockerfile'
contains "$TMP/publish" "build-args: CADDY_RUNTIME_IMAGE=\${{ needs.dependencies.outputs.caddy_runtime }}"
for file in restic-tools sops-tools socket-proxy-tools postgres-openssl cloudflared-tools keycloak-patched; do
    absent "$TMP/publish" "file: deployment/docker/$file"
done
pass 'application publication waits for all gates and never rebuilds dependency Dockerfiles'
for component in "${components[@]}"; do
    key=$(printf '%s_IMAGE' "$component" | tr '[:lower:]' '[:upper:]')
    contains "$TMP/publish" "$key: \${{ needs.dependencies.outputs.$component }}"
done
contains "$TMP/publish" "BACKEND_IMAGE: \${{ steps.names.outputs.backend }}@\${{ steps.backend.outputs.digest }}"
contains "$TMP/publish" "WEB_IMAGE: \${{ steps.names.outputs.web }}@\${{ steps.web.outputs.digest }}"
before "$TMP/publish" 'run: make audit-images' 'run: bash tools/quality/dependency-images/adopt.sh'
before "$TMP/publish" 'run: make audit-images' 'cosign sign --yes'
absent "$TMP/publish" 'AUDIT_IMAGES='
absent "$TMP/publish" 'AUDIT_APPLICATION_IMAGES=false'
pass 'complete exact-app/dependency audit precedes development adoption and application signing'

contains "$NIGHTLY" 'ref: dev'
contains "$NIGHTLY" 'make resolve-security-images'
before "$NIGHTLY" 'make resolve-security-images' 'make bootstrap'
before "$NIGHTLY" 'make resolve-security-images' 'make verify'
contains "$NIGHTLY" 'DOCKER_BUILD_FLAGS="--load'
contains "$NIGHTLY" 'security/image-reports/**/report.json'
contains "$NIGHTLY" 'security/image-reports/summary.tsv'
contains "$NIGHTLY" 'security/image-reports/db-snapshot.json'
absent "$NIGHTLY" 'make publish-security-images'
absent "$NIGHTLY" 'AUDIT_APPLICATION_IMAGES=false'
pass 'nightly explicitly checks out dev, resolves signed utilities and loads exact app artifacts'
contains "$TMP/security-audit" "ref: \${{ github.event_name == 'schedule' && 'dev' || github.sha }}"
contains "$TMP/security-audit" "if: github.event_name == 'push' && github.ref_name == 'dev'"
contains "$TMP/security-audit" 'run: make resolve-security-images'
contains "$TMP/security-audit" 'make audit-images AUDIT_APPLICATION_IMAGES=false'
contains "$TMP/security-audit" 'if: always()'
pass 'standalone Security preserves dependency-only scope while scheduled checkout targets dev'

MAKE_FRAGMENT="$ROOT/tools/make/dependency-images.mk"
contains "$MAKE_FRAGMENT" 'AUDIT_APPLICATION_IMAGES ?= true'
contains "$MAKE_FRAGMENT" 'for component in postgres cloudflared sops socket-proxy keycloak restic caddy-runtime; do'
contains "$MAKE_FRAGMENT" 'bash tools/quality/dependency-images/selected.sh'
contains "$MAKE_FRAGMENT" '!missing-selection-'
contains "$MAKE_FRAGMENT" "\$(BACKEND_IMAGE) \$(WEB_IMAGE)"
contains "$MAKE_FRAGMENT" 'deployment/images/runtime.tsv'
for target in audit-images audit-image-set; do
    declaration=$(awk -v name="$target" '$0 ~ "^"name":" {sub(/##.*/, ""); print}' "$ROOT/tools/make/"*.mk)
    [[ "$declaration" == "$target: "* || "$declaration" == "$target:" ]] || fail "missing scan-only $target rule"
    [[ "${declaration#*:}" != *[![:space:]]* ]] || fail "$target has preparation prerequisites"
done
count=0
while IFS=$'\t' read -r component ref; do
    [[ "$component" != \#* && -n "$component" ]] || continue
    [[ "$ref" =~ @sha256:[0-9a-f]{64}$ ]] || fail 'runtime inventory has a mutable reference'
    count=$((count + 1))
done <"$ROOT/deployment/images/runtime.tsv"
[[ "$count" == 8 ]] || fail 'runtime image scope changed without review'
for component in beszel beszel-agent victoria-logs victoria-metrics vector oauth2-proxy s3-fixture mailpit; do
    grep -q "^${component}[[:space:]]" "$ROOT/deployment/images/runtime.tsv" || fail "runtime component missing: $component"
done
pass 'default audit is scan-only and includes eight runtime pins, seven utilities and both apps'

contains "$TMP/release-promote" 'main does not contain the exact tested dev tree.'
contains "$TMP/release-promote" 'for component in postgres restic cloudflared caddy-runtime; do'
contains "$TMP/release-promote" "bash tools/quality/dependency-images/promote.sh \"\$component\" \"\$DEV_SHA\""
contains "$TMP/release-promote" "tools/quality/ci/promote-sops-image.sh \"\$DEV_SHA\""
contains "$TMP/release-promote" "tools/quality/ci/promote-socket-proxy-image.sh \"\$DEV_SHA\""
contains "$TMP/release-promote" 'run: tools/quality/ci/promote-application-images.sh'
for key in BACKEND WEB KEYCLOAK SOPS SOCKET_PROXY POSTGRES RESTIC CLOUDFLARED CADDY_RUNTIME; do
    contains "$TMP/release-promote" "$key"'_IMAGE:'
done
before "$TMP/release-promote" 'run: make audit-images' 'cosign sign --yes'
contains "$TMP/release-promote" "for image in \"\$BACKEND\" \"\$WEB\" \"\$KEYCLOAK\" \"\$SOPS\" \"\$SOCKET_PROXY\" \"\$POSTGRES\" \"\$RESTIC\" \"\$CLOUDFLARED\" \"\$CADDY_RUNTIME\"; do"
absent "$TMP/release-promote" 'AUDIT_IMAGES='
absent "$TMP/release-promote" 'AUDIT_APPLICATION_IMAGES=false'
absent "$TMP/release-promote" 'build-push-action@'
pass 'release promotes compatible source tree, scans complete promoted digests and signs all nine only afterward'
contains "$TMP/dev-deploy" 'bash tools/quality/dependency-images/install-host-tool.sh'
contains "$TMP/release-deploy" 'bash tools/quality/dependency-images/install-host-tool.sh'
contains "$TMP/dev-deploy" "\"deploy \$BACKEND \$WEB \$SOPS \$POSTGRES \$RESTIC \$GITHUB_SHA\""
contains "$TMP/release-deploy" "\"deploy \$BACKEND \$WEB \$KEYCLOAK \$SOPS \$POSTGRES \$RESTIC \$GITHUB_SHA\""
for file in "$ROOT/deployment/scripts/hosted/deploy.sh" "$ROOT/deployment/scripts/hosted/forced-command.sh"; do
    contains "$file" 'dev:7)'
    contains "$file" 'production:8)'
done
before "$ROOT/tools/quality/dependency-images/install-host-tool.sh" 'sha256sum --check' 'sudo dpkg -i'
contains "$ROOT/tools/quality/dependency-images/install-host-tool.sh" 'host-tools.json'
pass 'CI host installer validates reviewed checksum and new seven/eight-field SSH protocols align'

# Isolated real promotion script, fake Docker/Cosign, real input hashing/JSON.
FIXTURE="$TMP/repository"
mkdir -p "$FIXTURE/tools/quality/dependency-images" "$FIXTURE/tools/quality/image-audit" "$FIXTURE/deployment/images" "$TMP/bin"
cp "$ROOT/tools/quality/dependency-images/common.sh" "$ROOT/tools/quality/dependency-images/promote.sh" "$ROOT/tools/quality/dependency-images/registry.sh" "$FIXTURE/tools/quality/dependency-images/"
cp "$ROOT/tools/quality/image-audit/retry.sh" "$FIXTURE/tools/quality/image-audit/"
cp "$ROOT/deployment/images/dependencies.tsv" "$FIXTURE/deployment/images/"
cp "$ROOT/.dockerignore" "$FIXTURE/.dockerignore"
while IFS=$'\t' read -r component file _context _extras; do
    [[ "$component" != \#* && -n "$component" ]] || continue
    mkdir -p "$FIXTURE/$(dirname "$file")"
    cp "$ROOT/$file" "$FIXTURE/$file"
done <"$ROOT/deployment/images/dependencies.tsv"
export FIXTURE PATH="$TMP/bin:$PATH"
export DEV_SHA="$(printf 'a%.0s' {1..40})" GITHUB_SHA="$(printf 'b%.0s' {1..40})"
export TEST_DIGEST="sha256:$(printf 'c%.0s' {1..64})"
cat >"$TMP/bin/docker" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"$TRACE"
[[ "$1 $2" == 'buildx imagetools' ]] || exit 9
if [[ "$3" == create ]]; then
    [[ "$*" == *"--tag ghcr.io/anko59/geoguessme-$TEST_COMPONENT:release-$GITHUB_SHA"* ]] || exit 9
    [[ "${@: -1}" == "ghcr.io/anko59/geoguessme-$TEST_COMPONENT:dev-$DEV_SHA@$TEST_DIGEST" ]] || exit 9
    exit
fi
[[ "$3" == inspect ]] || exit 9
if [[ "$4" == *:dev-* && "${FAKE_PROMOTION:-}" == transient && ! -f "$STATE_DIR/retried" ]]; then
    : >"$STATE_DIR/retried"
    echo 'HTTP 429 Too Many Requests' >&2
    exit 1
fi
digest=$TEST_DIGEST
if [[ "$4" == *:release-* && "${FAKE_PROMOTION:-}" == changed ]]; then digest="sha256:$(printf 'd%.0s' {1..64})"; fi
if [[ "$4" == *:dev-* && "${FAKE_PROMOTION:-}" == invalid ]]; then digest=invalid; fi
printf '"%s"\n' "$digest"
FAKE
cat >"$TMP/bin/cosign" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'cosign %s\n' "$*" >>"$TRACE"
[[ "$1" == verify ]] || exit 9
. "$FIXTURE/tools/quality/dependency-images/common.sh"
load_component "$TEST_COMPONENT"
[[ "$*" == *'--certificate-oidc-issuer https://token.actions.githubusercontent.com'* ]] || exit 9
[[ "$*" == *"--annotations dependency-inputs=$INPUT_HASH"* ]] || exit 9
[[ "${@: -1}" == "ghcr.io/anko59/geoguessme-$TEST_COMPONENT:dev-$DEV_SHA@$TEST_DIGEST" ]] || exit 9
if [[ "$*" == *'--annotations revision='* ]]; then
    [[ "$*" == *"--annotations revision=$DEV_SHA"* ]] || exit 9
    [[ "$*" == *'--certificate-identity-regexp ^https://github.com/Anko59/GeoguessMe/\.github/workflows/deploy\.yml@refs/heads/dev$'* ]] || exit 9
    [[ "${FAKE_PROMOTION:-}" != wrong-revision && "${FAKE_PROMOTION:-}" != wrong-inputs ]] || exit 1
else
    [[ "$*" == *'--certificate-identity-regexp ^https://github.com/Anko59/GeoguessMe/\.github/workflows/(deploy|security)\.yml@refs/heads/dev$'* ]] || exit 9
    [[ "$*" == *'--annotations dependency-build=true'* ]] || exit 9
    [[ "${FAKE_PROMOTION:-}" != missing-build && "${FAKE_PROMOTION:-}" != wrong-identity ]] || exit 1
fi
FAKE
cat >"$TMP/bin/sleep" <<'FAKE'
#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >>"$TRACE"
FAKE
chmod +x "$TMP/bin/docker" "$TMP/bin/cosign" "$TMP/bin/sleep"
CASE=0
new_case() {
    CASE=$((CASE + 1))
    export TRACE="$TMP/trace-$CASE" GITHUB_OUTPUT="$TMP/output-$CASE" STATE_DIR="$TMP/state-$CASE"
    mkdir -p "$STATE_DIR"
    : >"$TRACE"
    : >"$GITHUB_OUTPUT"
    unset FAKE_PROMOTION
}
for component in postgres restic cloudflared caddy-runtime; do
    new_case
    export TEST_COMPONENT=$component
    bash "$FIXTURE/tools/quality/dependency-images/promote.sh" "$component" "$DEV_SHA" || fail 'compatible promotion failed'
    [[ "$(grep -c '^cosign verify ' "$TRACE")" == 2 ]] || fail 'missing independent trust checks'
    key=${component//-/_}
    contains "$GITHUB_OUTPUT" "$key=ghcr.io/anko59/geoguessme-$component:release-$GITHUB_SHA@$TEST_DIGEST"
    before "$TRACE" 'cosign verify ' 'docker buildx imagetools create '
    absent "$TRACE" 'docker build '
    absent "$TRACE" 'cosign sign '
    pass "$component promotion preserves digest and verifies revision/input hash/build-purpose signatures"
done
for error in changed invalid wrong-revision wrong-inputs missing-build wrong-identity; do
    new_case
    export TEST_COMPONENT=postgres FAKE_PROMOTION=$error
    if bash "$FIXTURE/tools/quality/dependency-images/promote.sh" postgres "$DEV_SHA" >"$TMP/result" 2>"$TMP/error"; then fail "promotion accepted $error"; fi
    [[ ! -s "$GITHUB_OUTPUT" ]] || fail 'failed promotion published outputs'
    absent "$TRACE" 'cosign sign '
    absent "$TRACE" 'sleep '
    if [[ "$error" != changed ]]; then absent "$TRACE" 'docker buildx imagetools create '; fi
    pass "promotion rejects $error without trusted output or signing"
done
new_case
export TEST_COMPONENT=postgres FAKE_PROMOTION=transient
bash "$FIXTURE/tools/quality/dependency-images/promote.sh" postgres "$DEV_SHA" >"$TMP/result" 2>"$TMP/error" || fail 'transient registry failure did not recover'
[[ "$(grep -c '^sleep ' "$TRACE")" == 1 ]] || fail 'missing bounded retry/backoff'
contains "$GITHUB_OUTPUT" "postgres=ghcr.io/anko59/geoguessme-postgres:release-$GITHUB_SHA@$TEST_DIGEST"
pass 'promotion recovers from a registry 429 without contaminating digest JSON or retrying policy failures'
if compgen -G "$FIXTURE/.local/.dependency-transport.*" >/dev/null; then fail 'transport session leaked temporary output'; fi
bash "$ROOT/tools/quality/dependency-images/test-consumers.sh"
printf 'security workflow contracts PASSED (%s checks)\n' "$PASS"
