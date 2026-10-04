#!/usr/bin/env bash
# Native pinned-Trivy report fixture: no database/registry request, no fake policy
# evaluator. Invoke through Make with its exported tooling namespace and uid/gid.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$ROOT"
: "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}"
: "${TOOLS_UID:?Run through Make}"
: "${TOOLS_GID:?Run through Make}"
mkdir -p security/image-reports
CACHE="$ROOT/security/image-reports/.trivy-cache"
[ ! -L "$CACHE" ] || {
    echo 'native policy: cache must not be a symlink' >&2
    exit 2
}
mkdir -p "$CACHE"
[ -w "$CACHE" ] || {
    echo 'native policy: caller cannot write the cache' >&2
    exit 2
}
TMP=$(mktemp -d "$ROOT/security/image-reports/.native-policy.XXXXXXXX")
cleanup() {
    case "$TMP" in "$ROOT/security/image-reports/.native-policy."*) rm -rf -- "$TMP" ;; *) return 1 ;; esac
}
trap cleanup EXIT
# Keep even native fixtures inside the restricted audit-artifact mount.
cp tools/quality/image-audit/fixed-high.json tools/quality/image-audit/fixed-critical.json "$TMP/"
trivy() {
    docker compose -p "$GEOGUESSME_TOOLS_PROJECT" -f deployment/compose.tools.yaml --project-directory . \
        run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" trivy trivy --config /dev/null "$@"
}
CHECKS=0
expect() {
    local expected=$1 rc=0
    shift
    "$@" >"$TMP/output" 2>&1 || rc=$?
    if [ "$rc" -ne "$expected" ]; then
        cat "$TMP/output"
        echo "native policy exit $rc != $expected" >&2
        exit 1
    fi
    CHECKS=$((CHECKS + 1))
}
expect 0 docker compose -p "$GEOGUESSME_TOOLS_PROJECT" -f deployment/compose.tools.yaml --project-directory . \
    run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" --entrypoint /bin/sh trivy \
    -ec 'test -d /workspace/security/image-reports; test ! -e /workspace/.git; test ! -e /workspace/deployment; test ! -e /root/.docker'
# Probe the mounted cache as TOOLS_UID, not container root, without touching DB
# contents. The only deleted file is this exact, allocated cache probe.
# The probe variables expand in the container shell, not in this host harness.
# shellcheck disable=SC2016
expect 0 docker compose -p "$GEOGUESSME_TOOLS_PROJECT" -f deployment/compose.tools.yaml --project-directory . \
    run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" --entrypoint /bin/sh trivy \
    -ec 'probe=$(mktemp /tmp/trivy-cache/.native-policy.XXXXXXXX); case "$probe" in /tmp/trivy-cache/.native-policy.????????) ;; *) exit 2 ;; esac; printf "writable\n" >"$probe"; rm -f -- "${probe:?}"'
checker=tools/quality/image-scan-exceptions-check.sh
ref="fixture:base@sha256:$(printf 'a%.0s' {1..64})"
expiry_epoch=$(($(date -u +%s) + 7 * 86400))
expiry=$(date -u -d "@$expiry_epoch" +%F 2>/dev/null || date -u -r "$expiry_epoch" +%F)
printf '# no reviewed exceptions\n' >"$TMP/exceptions.yaml"
export IMAGE_SCAN_EXCEPTIONS="$TMP/exceptions.yaml"
bash "$checker" --emit-policy "$ref" "$TMP/policy.rego"
policy="/workspace/${TMP#"$ROOT/"}/policy.rego"
for fixture in fixed-high fixed-critical; do
    expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
        --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/$fixture.json"
done
cat >"$TMP/exceptions.yaml" <<EOF
- id: CVE-2026-103111
  image: $ref
  digest: sha256:$(printf 'a%.0s' {1..64})
  package: pcre2
  installed_version: 10.48-r0
  owner: regression-fixture
  reachable: native fixed High filtering test only
  approved: true
  expires: $expiry
EOF
bash "$checker" --emit-policy "$ref" "$TMP/policy.rego"
expect 0 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/fixed-high.json"
# Same CVE, different package version must NOT inherit an old package exception.
sed 's/10.48-r0/10.49-r0/' tools/quality/image-audit/fixed-high.json >"$TMP/changed.json"
expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/changed.json"
sed 's/"PkgName": "pcre2"/"PkgName": "different-package"/' tools/quality/image-audit/fixed-high.json >"$TMP/changed.json"
expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/changed.json"
# Missing package metadata must not fall back to a CVE-wide exemption.
sed '/"PkgName":/d; /"InstalledVersion":/d' tools/quality/image-audit/fixed-high.json >"$TMP/changed.json"
expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/changed.json"
# A reviewed High exception cannot suppress a distinct Critical vulnerability.
expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/fixed-critical.json"
echo "native Trivy policy/isolation/cache regression tests PASSED ($CHECKS checks)"
