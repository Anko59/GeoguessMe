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
policy="/workspace/${TMP#"$ROOT/"}/policy.rego"
printf '{"vulnerabilities":[{"cveID":"CVE-2021-44228"}]}\n' >"$TMP/kev.json"
docker compose -p "$GEOGUESSME_TOOLS_PROJECT" -f deployment/compose.tools.yaml --project-directory . \
    run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" go-security \
    bash /workspace/tools/quality/image-audit/risk-policy.sh emit "/workspace/${TMP#"$ROOT/"}/kev.json" >"$TMP/policy.rego"
expect 0 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/fixed-high.json"
expect 42 trivy convert --severity HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/fixed-critical.json"
# Known exploitation blocks even at LOW severity with no published fix.
sed 's/"Severity": "CRITICAL"/"Severity": "LOW"/; s/"FixedVersion": "[^"]*"/"FixedVersion": ""/' \
    tools/quality/image-audit/fixed-critical.json >"$TMP/known-unfixed.json"
expect 42 trivy convert --severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/known-unfixed.json"
sed 's/"VulnerabilityID": "CVE-2021-44228"/"VulnerabilityID": "GHSA-fixture", "VendorIDs": ["CVE-2021-44228"]/' \
    tools/quality/image-audit/fixed-critical.json >"$TMP/known-alias.json"
expect 42 trivy convert --severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL --exit-code 42 --ignorefile /dev/null \
    --ignore-policy "$policy" --format table "/workspace/${TMP#"$ROOT/"}/known-alias.json"
echo "native exploitation policy tests PASSED ($CHECKS total checks)"
