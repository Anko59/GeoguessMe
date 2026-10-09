#!/usr/bin/env bash
# Deterministic lifecycle regressions. Run through make test-dependency-images.
# All Docker, Cosign, Make and retry delays are fakes; no registry or daemon.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TMP=$(mktemp -d)
[[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] || exit 1
cleanup() { [[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] && rm -rf -- "$TMP"; }
trap cleanup EXIT
FIXTURE="$TMP/repository"
mkdir -p "$FIXTURE/tools/quality/dependency-images" "$FIXTURE/tools/quality/image-audit" "$FIXTURE/deployment/images" "$TMP/bin"
cp "$ROOT/tools/quality/dependency-images/"*.sh "$FIXTURE/tools/quality/dependency-images/"
cp -R "$ROOT/tools/quality/dependency-images/test" "$FIXTURE/tools/quality/dependency-images/"
cp "$ROOT/tools/quality/image-audit/retry.sh" "$FIXTURE/tools/quality/image-audit/"
cp "$ROOT/deployment/images/dependencies.tsv" "$FIXTURE/deployment/images/"
cp "$ROOT/.dockerignore" "$FIXTURE/.dockerignore"
while IFS=$'\t' read -r name file _context _extras; do
    [[ "$name" != \#* && -n "$name" ]] || continue
    mkdir -p "$FIXTURE/$(dirname "$file")"
    cp "$ROOT/$file" "$FIXTURE/$file"
done <"$ROOT/deployment/images/dependencies.tsv"
# Never inherit caller-selected artifacts or write test results to real CI outputs.
unset KEYCLOAK_IMAGE RESTIC_IMAGE SOPS_IMAGE SOCKET_PROXY_IMAGE POSTGRES_IMAGE CLOUDFLARED_IMAGE CADDY_RUNTIME_IMAGE GITHUB_OUTPUT
export FIXTURE
export PATH="$TMP/bin:$PATH"
export TEST_COMPONENT=sops BUILDX_BUILDER=isolated-cache-builder
export TEST_DIGEST="sha256:$(printf 'd%.0s' {1..64})"
export TEST_IMAGE_ID="sha256:$(printf 'c%.0s' {1..64})"

cat >"$TMP/bin/docker" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"$TRACE"
. "$FIXTURE/tools/quality/dependency-images/common.sh"
load_component "$TEST_COMPONENT"
if [[ "$1 $2" == 'context show' ]]; then echo fixture-context; exit; fi
if [[ "$1 $2" == 'image tag' ]]; then
    [[ "$3" == "$TEST_IMAGE_ID" ]] || exit 9
    printf '%s\n' "$3" >"$FAKE_STATE/alias"
    exit
fi
if [[ "$1 $2" == 'image inspect' ]]; then
    ref=$3
    [[ "${FAKE_DAEMON:-}" != broken ]] || { echo 'Cannot connect to Docker daemon' >&2; exit 1; }
    [[ "${FAKE_ID_MISSING:-}" != yes || "$ref" != "$TEST_IMAGE_ID" ]] || { echo "Error: No such image: $ref" >&2; exit 1; }
    [[ -f "$FAKE_STATE/local-$INPUT_HASH" || "$ref" == *@* || ("$ref" == "$TEST_IMAGE_ID" && -f "$FAKE_STATE/image-hash") ]] || { echo "Error: No such image: $ref" >&2; exit 1; }
    id=$TEST_IMAGE_ID
    if [[ "$ref" == *:config-* ]]; then
        [[ -f "$FAKE_STATE/alias" ]] || { echo "Error: No such image: $ref" >&2; exit 1; }
        id=$(<"$FAKE_STATE/alias")
    elif [[ "$ref" == "$LOCAL_REF" && "${FAKE_RETAG:-}" == changed ]]; then id="sha256:$(printf 'b%.0s' {1..64})"; fi
    if [[ "$*" == *'.RepoDigests'* ]]; then
        digest=$TEST_DIGEST
        [[ "${FAKE_DIGEST:-}" != wrong ]] || digest="sha256:$(printf 'b%.0s' {1..64})"
        jq -n --arg ref "ghcr.io/anko59/geoguessme-sops@$digest" '[$ref]'
        exit
    fi
    hash=$INPUT_HASH
    if [[ "$ref" == "$TEST_IMAGE_ID" || "$ref" == *:config-* ]]; then hash=$(<"$FAKE_STATE/image-hash"); fi
    [[ "${FAKE_LABEL:-}" != wrong ]] || hash=wrong
    name=$FINAL_BASE_NAME
    [[ "${FAKE_BASE:-}" != wrong ]] || name=other
    jq -n --arg id "$id" --arg hash "$hash" --arg name "$name" --arg digest "$FINAL_BASE_DIGEST" --arg arch "${FAKE_ARCH:-amd64}" '
        [{Id:$id, Os:"linux", Architecture:$arch, Config:{Labels:{
            "dev.geoguessme.dependency-inputs":$hash,
            "org.opencontainers.image.base.name":$name,
            "org.opencontainers.image.base.digest":$digest}}}]
    '
    exit
fi
if [[ "$1" == build || "$1 $2" == 'buildx build' ]]; then
    metadata='' iidfile=''
    args=("$@")
    for ((i=0; i<${#args[@]}; i++)); do
        if [[ "${args[i]}" == --metadata-file ]]; then metadata=${args[i+1]}; fi
        if [[ "${args[i]}" == --iidfile ]]; then iidfile=${args[i+1]}; fi
    done
    [[ "$1" != build || "${BUILDX_BUILDER:-}" == fixture-context ]] || { echo 'local build was not loaded into the active daemon' >&2; exit 9; }
    [[ "$*" == *'--platform linux/amd64'* ]] || exit 9
    [[ "$*" == *"DEPENDENCY_INPUTS=$INPUT_HASH"* ]] || exit 9
    [[ "$*" == *"--label org.opencontainers.image.base.name=$FINAL_BASE_NAME"* ]] || exit 9
    [[ "$*" == *"--label org.opencontainers.image.base.digest=$FINAL_BASE_DIGEST"* ]] || exit 9
    : >"$FAKE_STATE/local-$INPUT_HASH"
    printf '%s\n' "$INPUT_HASH" >"$FAKE_STATE/image-hash"
    if [[ -n "$iidfile" ]]; then printf '%s\n' "$TEST_IMAGE_ID" >"$iidfile"; fi
    if [[ -n "$metadata" ]]; then
        jq -n --arg digest "$TEST_DIGEST" '{"containerimage.digest":$digest}' >"$metadata"
        : >"$FAKE_STATE/published"
    fi
    exit
fi
if [[ "$1 $2 $3" == 'buildx imagetools create' ]]; then exit; fi
if [[ "$1 $2 $3" == 'buildx imagetools inspect' ]]; then
    if [[ "${FAKE_ADOPTION:-}" == wrong && "$4" == *:dev-* ]]; then
        printf '"sha256:%s"\n' "$(printf 'b%.0s' {1..64})"
        exit
    fi
    if [[ "$*" == *'--raw'* ]]; then
        if [[ "${FAKE_PROVENANCE:-}" == absent ]]; then echo '{"manifests":[]}'; exit; fi
        child="sha256:$(printf 'a%.0s' {1..64})"
        jq -n --arg child "$child" '{manifests:[
            {digest:$child,platform:{os:"linux",architecture:"amd64"}},
            {digest:"attestation",platform:{os:"unknown",architecture:"unknown"},annotations:{
                "vnd.docker.reference.type":"attestation-manifest", "vnd.docker.reference.digest":$child}}]} | if env.FAKE_PROVENANCE == "direct-multi-runtime" then .manifests += [{digest:"other-runtime",platform:{os:"linux",architecture:"arm64"}}] else . end'
        exit
    fi
    if [[ "$*" == *'.Provenance'* ]]; then
        bash "$FIXTURE/tools/quality/dependency-images/test/provenance-fixture.sh" "$INPUT_HASH" "$FINAL_BASE_DIGEST"
        exit
    fi
    case "${FAKE_REGISTRY:-existing}" in
        missing)
            if [[ ! -f "$FAKE_STATE/published" ]]; then echo 'MANIFEST_UNKNOWN: manifest unknown (request abc429500deadbeef)' >&2; exit 1; fi
            ;;
        rate) echo 'HTTP 429 Too Many Requests' >&2; exit 1 ;;
        auth) echo 'HTTP 401 unauthorized' >&2; exit 1 ;;
        unknown) echo 'arbitrary client failure' >&2; exit 1 ;;
        server) echo 'HTTP 501 Not Implemented: MANIFEST_UNKNOWN' >&2; exit 1 ;;
        mixed) echo 'HTTP 429 then HTTP 401 unauthorized' >&2; exit 1 ;;
        malformed) echo '"not-a-digest"'; exit ;;
    esac
    printf '"%s"\n' "$TEST_DIGEST"
    exit
fi
[[ "$1" == pull ]] && exit
printf 'unexpected Docker operation: %s\n' "$*" >&2
exit 9
FAKE
cat >"$TMP/bin/cosign" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'cosign %s\n' "$*" >>"$TRACE"
case "$1" in
    verify)
        [[ "${FAKE_SIGNATURE:-}" != bad ]] || { echo 'no matching signature' >&2; exit 1; }
        [[ "$*" == *'dependency-inputs='* && "$*" == *'token.actions.githubusercontent.com'* ]] || exit 9
        [[ "$*" == *'/workflows/(deploy|security)\.yml@refs/heads/dev$'* && "$*" == *'dependency-build=true'* ]] || exit 9
        ;;
    sign)
        [[ -f "$FAKE_STATE/scanned" ]] || { echo 'signed before scanning' >&2; exit 9; }
        if [[ "$*" == *'revision='* ]]; then
            [[ "$*" != *'dependency-build=true'* ]] || exit 9
        else
            [[ "$*" == *'dependency-build=true'* ]] || exit 9
        fi
        : >"$FAKE_STATE/signed"
        ;;
    *) exit 9 ;;
esac
FAKE
cat >"$TMP/bin/make" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'make %s\n' "$*" >>"$TRACE"
[[ "$*" == *'audit-image-set'* && "$*" == *"IMAGE_AUDIT_REFS="*@"$TEST_DIGEST"* ]] || exit 9
[[ "${FAKE_SCAN:-}" != vulnerability ]] || { echo 'known-exploited vulnerability' >&2; exit 42; }
: >"$FAKE_STATE/scanned"
FAKE
cat >"$TMP/bin/sleep" <<'FAKE'
#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >>"$TRACE"
FAKE
chmod +x "$TMP/bin/"*
PASS=0
CASE=0
new_case() {
    CASE=$((CASE + 1))
    export FAKE_STATE="$TMP/case-$CASE"
    mkdir -p "$FAKE_STATE"
    export TRACE="$FAKE_STATE/trace"
    : >"$TRACE"
    unset FAKE_REGISTRY FAKE_SIGNATURE FAKE_PROVENANCE FAKE_LABEL FAKE_BASE FAKE_DAEMON FAKE_DIGEST FAKE_SCAN FAKE_ADOPTION FAKE_ARCH FAKE_ID_MISSING FAKE_RETAG
    ENV_FILE="$FIXTURE/.local/security-images.env"
    [[ "$ENV_FILE" == "$FIXTURE/.local/security-images.env" && ! -L "$ENV_FILE" ]] || exit 1
    rm -f -- "$ENV_FILE"
}
pass() {
    PASS=$((PASS + 1))
    printf 'PASS: %s\n' "$*"
}
assert() { "$@" || {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}; }
run_ok() {
    bash "$FIXTURE/tools/quality/dependency-images/lifecycle.sh" "$@" >"$FAKE_STATE/output" 2>"$FAKE_STATE/log" || {
        sed -n '1,100p' "$FAKE_STATE/log" >&2
        exit 1
    }
}
run_fail() {
    if bash "$FIXTURE/tools/quality/dependency-images/lifecycle.sh" "$@" >"$FAKE_STATE/output" 2>"$FAKE_STATE/log"; then
        echo 'FAIL: expected lifecycle rejection' >&2
        exit 1
    fi
}
no_build_or_sign() { ! grep -Eq '^docker (build |buildx build)|^cosign sign ' "$TRACE"; }
ref() { bash "$FIXTURE/tools/quality/dependency-images/image-ref.sh" local sops; }
valid_content_ref() { [[ "$1" =~ ^geoguessme/[a-z-]+:dependency-[0-9a-f]{64}$ ]]; }

new_case
first=$(ref)
assert test "$first" = "$(ref)"
assert test "$first" = "$(GITHUB_SHA=other ref)"
chmod 0640 "$FIXTURE/.dockerignore"
assert test "$first" = "$(ref)"
assert test ! -s "$TRACE"
pass 'pure stable hash excludes application revision and all network/daemon calls'
for component in keycloak restic sops socket-proxy postgres cloudflared caddy-runtime; do
    value=$(bash "$FIXTURE/tools/quality/dependency-images/image-ref.sh" local "$component")
    assert valid_content_ref "$value"
done
pass 'all seven reviewed components have valid content identities'

run_ok prepare-local sops
assert grep -Fxq "SOPS_IMAGE=$TEST_IMAGE_ID" "$ENV_FILE"
assert test "$(grep -Ec '^docker build ' "$TRACE")" = 1
mkdir -p "$FIXTURE/frontend"
printf 'frontend change\n' >"$FIXTURE/frontend/feature.ts"
run_ok prepare-local sops
assert test "$first" = "$(ref)"
assert test "$(grep -Ec '^docker build ' "$TRACE")" = 1
pass 'unchanged dependency reused after frontend-only modification; local output immutable ID'

printf '\n# reviewed change\n' >>"$FIXTURE/deployment/docker/sops-tools/Dockerfile"
assert test "$first" != "$(ref)"
run_ok prepare-local sops
assert test "$(grep -Ec '^docker build ' "$TRACE")" = 2
pass 'changed Dockerfile produces new key and one explicit preparation build'
previous=$(ref)
printf '\n# changed context filter\n' >>"$FIXTURE/.dockerignore"
assert test "$previous" != "$(ref)"
pass 'context ignore configuration participates in identity'

selected() { bash "$FIXTURE/tools/quality/dependency-images/selected.sh" sops "$@"; }
selected_fail() { if selected "$@" >"$FAKE_STATE/output" 2>"$FAKE_STATE/log"; then
    echo 'FAIL: expected immutable consumer rejection' >&2
    exit 1
fi; }
selected_case() {
    new_case
    run_ok prepare-local sops
    : >"$TRACE"
}
new_case
export FAKE_RETAG=changed
run_ok prepare-local sops
assert grep -Fxq "SOPS_IMAGE=$TEST_IMAGE_ID" "$ENV_FILE"
pass 'explicit local build captures its own IID even if a concurrent builder retags the output'
: >"$TRACE"
SOPS_IMAGE="$TEST_IMAGE_ID" run_ok prepare-local sops
assert no_build_or_sign
assert test "$(awk '/^docker image inspect geoguessme\/sops:dependency-/ {n++} END {print n+0}' "$TRACE")" = 0
pass 'explicit local immutable selection is preserved during preparation without tag lookup'
selected_case
export FAKE_RETAG=changed
assert test "$(selected)" = "$TEST_IMAGE_ID"
assert no_build_or_sign
assert test "$(awk '/^docker image inspect geoguessme\/sops:dependency-/ {n++} END {print n+0}' "$TRACE")" = 0
pass 'consumer keeps prepared immutable ID after another checkout retags the input-key tag'
alias="geoguessme/sops:config-${TEST_IMAGE_ID#sha256:}"
assert test "$(selected build)" = "$alias"
assert test "$(selected build)" = "$alias"
assert test "$(grep -c '^docker image tag ' "$TRACE")" = 1
pass 'local FROM gets a config-ID-addressed alias created once from exact saved bytes'
printf 'sha256:%s\n' "$(printf 'b%.0s' {1..64})" >"$FAKE_STATE/alias"
selected_fail build
assert test "$(grep -c '^docker image tag ' "$TRACE")" = 1
pass 'conflicting config-addressed alias is rejected without retagging'
for error in unknown duplicate injected missing unterminated; do
    selected_case
    case "$error" in
        unknown) printf 'UNREVIEWED_IMAGE=%s\n' "$TEST_IMAGE_ID" >>"$ENV_FILE" ;;
        duplicate) printf 'SOPS_IMAGE=%s\n' "$TEST_IMAGE_ID" >>"$ENV_FILE" ;;
        injected) printf '%s\n' "RESTIC_IMAGE=\$(touch \"$FIXTURE/untrusted\")" >>"$ENV_FILE" ;;
        missing) printf 'RESTIC_IMAGE=%s\n' "$TEST_IMAGE_ID" >"$ENV_FILE" ;;
        unterminated) printf 'UNREVIEWED_IMAGE=invalid' >>"$ENV_FILE" ;;
    esac
    selected_fail
    assert no_build_or_sign
    assert test ! -e "$FIXTURE/untrusted"
    pass "consumer rejects $error saved environment without sourcing it or rebuilding"
done
selected_case
export FAKE_ARCH=arm64
selected_fail
assert no_build_or_sign
pass 'consumer rejects prepared bytes for the wrong platform'
selected_case
export FAKE_ID_MISSING=yes
selected_fail
assert no_build_or_sign
pass 'removed prepared image fails rather than falling back to a retagged cache entry'
selected_case
printf '\n# changed input after preparation\n' >>"$FIXTURE/deployment/docker/sops-tools/Dockerfile"
selected_fail
assert no_build_or_sign
pass 'stale saved image cannot satisfy a changed reviewed dependency input key'
selected_case
printf 'UNREVIEWED_IMAGE=bad\n' >"$ENV_FILE"
remote="$(bash "$FIXTURE/tools/quality/dependency-images/image-ref.sh" remote sops)@$TEST_DIGEST"
assert test "$(SOPS_IMAGE="$remote" selected build)" = "$remote"
assert no_build_or_sign
pass 'explicit CI registry digest outranks unrelated saved state and stays unchanged for FROM'
new_case
export FAKE_DAEMON=broken
assert test "$(SOPS_IMAGE="$remote" selected audit)" = "$remote"
assert test ! -s "$TRACE"
pass 'fresh CI audit forwards trusted immutable refs without daemon inspection or network'
stale="ghcr.io/anko59/geoguessme-sops:dependency-$(printf '%064d' 0)@$TEST_DIGEST"
SOPS_IMAGE="$stale" selected_fail audit
assert test ! -s "$TRACE"
pass 'audit rejects a stale content input key without pulling or building'
selected_case
export FAKE_LABEL=wrong
selected_fail audit
assert no_build_or_sign
pass 'audit local saved IDs retain full current-key validation'

new_case
run_ok prepare-local sops
export FAKE_LABEL=wrong
run_fail prepare-local sops
assert test "$(grep -Ec '^docker build ' "$TRACE")" = 1
pass 'conflicting cached input label rejected without rebuilding'
new_case
export FAKE_DAEMON=broken
run_fail prepare-local sops
assert no_build_or_sign
pass 'daemon outage never masquerades as cache miss'
new_case
run_ok prepare-local sops
export FAKE_BASE=wrong
run_fail prepare-local sops
assert test "$(grep -Ec '^docker build ' "$TRACE")" = 1
pass 'cached upstream provenance mismatch rejected'

new_case
run_ok resolve sops
assert no_build_or_sign
assert grep -Eq '^SOPS_IMAGE=ghcr.io/anko59/geoguessme-sops:dependency-[0-9a-f]{64}@sha256:[0-9a-f]{64}$' "$ENV_FILE"
assert grep -q '^docker pull --platform linux/amd64 .*@sha256:' "$TRACE"
pass 'resolve verifies and pulls exact existing signed artifact, never builds or signs'

for error in missing rate auth unknown server mixed malformed; do
    new_case
    export FAKE_REGISTRY=$error
    run_fail resolve sops
    assert no_build_or_sign
    assert test ! -f "$ENV_FILE"
    if [[ "$error" == rate ]]; then assert test "$(grep -Ec '^sleep ' "$TRACE")" = 3; fi
    if [[ "$error" == auth || "$error" == mixed ]]; then assert test "$(grep -Ec '^sleep ' "$TRACE")" = 0; fi
    pass "resolve fails closed on $error without output publication or rebuild"
done
for error in rate auth unknown server mixed; do
    new_case
    export FAKE_REGISTRY=$error
    run_fail publish sops
    assert no_build_or_sign
    pass "publish cannot treat $error as an absent artifact"
done
new_case
export FAKE_SIGNATURE=bad
run_fail publish sops
assert no_build_or_sign
pass 'existing invalid signature cannot trigger replacement build'
for provenance in absent missing-slsa; do
    new_case
    export FAKE_PROVENANCE=$provenance
    run_fail resolve sops
    assert no_build_or_sign
    pass "existing artifact requires $provenance provenance validation"
done
new_case
export FAKE_DIGEST=wrong
run_fail resolve sops
assert no_build_or_sign
pass 'pulled content must preserve signed registry digest'

new_case
export FAKE_REGISTRY=missing
run_ok publish sops
assert test "$(grep -Ec '^docker buildx build ' "$TRACE")" = 1
assert test "$(grep -Ec '^cosign sign ' "$TRACE")" = 1
scan_line=$(grep -n '^make ' "$TRACE" | cut -d: -f1)
sign_line=$(grep -n '^cosign sign ' "$TRACE" | cut -d: -f1)
assert test "$scan_line" -lt "$sign_line"
assert grep -q -- '--push --sbom=true --provenance=mode=max' "$TRACE"
assert grep -q "@$TEST_DIGEST" "$ENV_FILE"
run_ok publish sops
assert test "$(grep -Ec '^docker buildx build ' "$TRACE")" = 1
pass 'explicit missing publication scans exact output before signing and then reuses it'

new_case
export FAKE_REGISTRY=missing FAKE_SCAN=vulnerability
run_fail publish sops
assert test "$(grep -Ec '^cosign sign ' "$TRACE")" = 0
assert test ! -f "$ENV_FILE"
pass 'genuine vulnerability failure prevents signing and state publication'
new_case
export FAKE_REGISTRY=missing FAKE_PROVENANCE=absent
run_fail publish sops
assert test "$(grep -Ec '^cosign sign ' "$TRACE")" = 0
pass 'new artifact requires provenance before signing'
# shellcheck source=tools/quality/dependency-images/test/provenance-contracts.sh
. "$ROOT/tools/quality/dependency-images/test/provenance-contracts.sh"

new_case
export GITHUB_OUTPUT="$FAKE_STATE/github-output"
run_ok resolve sops
assert grep -Eq '^sops=.*@sha256:[0-9a-f]{64}$' "$GITHUB_OUTPUT"
assert test "$(stat -c %a "$ENV_FILE")" = 600
pass 'validated output published atomically with private local permissions and GitHub output'
unset GITHUB_OUTPUT
new_case
mkdir -p "$FIXTURE/.local"
printf 'RESTIC_IMAGE=%s' "$TEST_IMAGE_ID" >"$ENV_FILE"
run_ok prepare-local sops
assert grep -Fxq "RESTIC_IMAGE=$TEST_IMAGE_ID" "$ENV_FILE"
pass 'subset preparation preserves an unrequested last selection without a final newline'
new_case
printf 'SOPS_IMAGE=previous\n' >"$ENV_FILE"
export FAKE_SIGNATURE=bad
run_fail resolve sops
assert grep -Fxq 'SOPS_IMAGE=previous' "$ENV_FILE"
pass 'failed resolution leaves previous environment untouched'
new_case
mkdir -p "$FIXTURE/.local/.dependency-environment.lock"
printf 'RESTIC_IMAGE=%s\n' "$TEST_IMAGE_ID" >"$ENV_FILE"
run_fail resolve sops
assert grep -Fxq "RESTIC_IMAGE=$TEST_IMAGE_ID" "$ENV_FILE"
assert test -d "$FIXTURE/.local/.dependency-environment.lock"
assert no_build_or_sign
rmdir "$FIXTURE/.local/.dependency-environment.lock"
pass 'concurrent environment merge fails closed and preserves another writer lock/state'

# Adoption adds revision approval, never changes original dependency bytes.
setup_adoption() {
    export GITHUB_SHA="$(printf 'e%.0s' {1..40})" GITHUB_REF=refs/heads/dev
    export GITHUB_OUTPUT="$FAKE_STATE/github-output"
    for component in keycloak restic sops socket-proxy postgres cloudflared caddy-runtime; do
        remote=$(bash "$FIXTURE/tools/quality/dependency-images/image-ref.sh" remote "$component")
        key=$(printf '%s_IMAGE' "$component" | tr '[:lower:]-' '[:upper:]_')
        export "$key=$remote@$TEST_DIGEST"
    done
    : >"$FAKE_STATE/scanned"
}
new_case
setup_adoption
bash "$FIXTURE/tools/quality/dependency-images/adopt.sh" >"$FAKE_STATE/output" 2>"$FAKE_STATE/log" || {
    sed -n '1,100p' "$FAKE_STATE/log" >&2
    exit 1
}
assert test "$(grep -Ec '^cosign sign ' "$TRACE")" = 7
assert test "$(grep -Ec "^.*=ghcr.io/anko59/geoguessme-.*:dev-$GITHUB_SHA@$TEST_DIGEST$" "$GITHUB_OUTPUT")" = 7
assert grep -q -- "revision=$GITHUB_SHA.*dependency-inputs=" "$TRACE"
pass 'adoption preserves all seven original digests and adds revision approval signatures'
new_case
setup_adoption
export FAKE_ADOPTION=wrong
if bash "$FIXTURE/tools/quality/dependency-images/adopt.sh" >"$FAKE_STATE/output" 2>"$FAKE_STATE/log"; then
    echo 'FAIL: adoption accepted changed digest' >&2
    exit 1
fi
assert test "$(grep -Ec '^cosign sign ' "$TRACE")" = 0
assert test ! -s "$GITHUB_OUTPUT"
pass 'adoption rejects a changed manifest digest before signing or publishing output'
unset KEYCLOAK_IMAGE RESTIC_IMAGE SOPS_IMAGE SOCKET_PROXY_IMAGE POSTGRES_IMAGE CLOUDFLARED_IMAGE CADDY_RUNTIME_IMAGE GITHUB_OUTPUT

# Vendor envelopes may not grow package patches or source rebuilds.
new_case
original="$FIXTURE/deployment/docker/sops-tools/Dockerfile"
cp "$original" "$TMP/envelope"
for instruction in 'RUN apk upgrade' 'COPY helper.txt /opt/helper.txt' 'ADD helper.txt /opt/helper.txt' 'ONBUILD RUN true' 'FROM pinned:test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; do
    cp "$TMP/envelope" "$original"
    printf '\n%s\n' "$instruction" >>"$original"
    if ref >"$FAKE_STATE/output" 2>"$FAKE_STATE/log"; then
        echo "FAIL: upstream ownership escaped through $instruction" >&2
        exit 1
    fi
    assert test ! -s "$TRACE"
    pass "vendor envelope rejects $instruction before external operations"
done
cp "$TMP/envelope" "$original"
printf '\nFROM floating:latest\n' >>"$original"
if ref >"$FAKE_STATE/output" 2>"$FAKE_STATE/log"; then
    echo 'FAIL: unpinned base accepted' >&2
    exit 1
fi
assert test ! -s "$TRACE"
pass 'unpinned upstream rejected before daemon or registry access'
printf 'dependency image regressions PASSED (%s checks)\n' "$PASS"
