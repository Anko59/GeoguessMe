#!/usr/bin/env bash
# Run through make test-image-audit, in the pinned Bash tooling container.
set -euo pipefail
SOURCE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
TMP=$(mktemp -d)
cleanup() {
    case "$TMP" in /tmp/tmp.*) rm -rf -- "$TMP" ;; *)
        echo 'unexpected test temp path' >&2
        return 1
        ;;
    esac
}
trap cleanup EXIT
mkdir -p "$TMP/bin" "$TMP/repo/tools/quality/image-audit" "$TMP/state"
cp "$SOURCE/audit.sh" "$SOURCE/retry.sh" "$SOURCE/risk-policy.sh" "$TMP/repo/tools/quality/image-audit/"
cp "$SOURCE/blocking-cves.tsv" "$TMP/repo/tools/quality/image-audit/"
cp "$SOURCE/fake-docker.sh" "$TMP/bin/fake-docker-real"
# Assert the bind source exists and is caller-owned before Docker can create it.
cat >"$TMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
cache="$FAKE_ROOT/security/image-reports/.trivy-cache"
if [ ! -d "$cache" ] || [ ! -w "$cache" ] || [ "$(stat -c '%u' "$cache")" != "$(id -u)" ]; then
    echo 'fake Docker: caller-owned writable cache must already exist' >&2
    exit 9
fi
probe=$(mktemp "$cache/.fake-cache.XXXXXXXX")
case "$probe" in "$cache"/.fake-cache.????????) rm -f -- "${probe:?}" ;; *) exit 9 ;; esac
printf 'cache ready before Docker\n' >"$FAKE_STATE/cache-ready"
exec "${BASH_SOURCE[0]%/*}/fake-docker-real" "$@"
DOCKER
# Expand these variables in the generated sleep fixture, not this test process.
# shellcheck disable=SC2016
printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$1" >>"$FAKE_STATE/delays"\n' >"$TMP/bin/sleep"
chmod +x "$TMP/bin/docker" "$TMP/bin/fake-docker-real" "$TMP/bin/sleep"
export PATH="$TMP/bin:$PATH" FAKE_STATE="$TMP/state" FAKE_ROOT="$TMP/repo" FAKE_FIXTURES="$SOURCE"
export GEOGUESSME_TOOLS_PROJECT=fixture TOOLS_UID=1000 TOOLS_GID=1000
export IMAGE_SCAN_EXCEPTIONS="$TMP/exceptions.yaml"
cd "$TMP/repo"
CHECKS=0
assert() {
    if ! "$@"; then
        cat "$TMP/output" "$FAKE_STATE/trace" >&2
        echo "FAIL: $*" >&2
        exit 1
    fi
    CHECKS=$((CHECKS + 1))
}
reset() {
    # Every deletion target is one of the explicitly allocated test children.
    for path in "$TMP/state" "$TMP/repo/security"; do
        case "$path" in "$TMP/state" | "$TMP/repo/security") rm -rf -- "$path" ;; *) exit 1 ;; esac
    done
    mkdir -p "$TMP/state"
    printf '# no exceptions\n' >"$IMAGE_SCAN_EXCEPTIONS"
    export FAKE_SCENARIO=$1
}
run() {
    local expected=$1 refs=$2 rc=0
    IMAGE_AUDIT_REFS="$refs" bash tools/quality/image-audit/audit.sh >"$TMP/output" 2>&1 || rc=$?
    if [ "$rc" -ne "$expected" ]; then
        cat "$TMP/output"
        echo "FAIL: exit $rc != $expected" >&2
        exit 1
    fi
    CHECKS=$((CHECKS + 1))
}
count_is() { [ -f "$FAKE_STATE/$1" ] && [ "$(<"$FAKE_STATE/$1")" = "$2" ]; }
ref="remote/image@sha256:$(printf 'f%.0s' {1..64})"

reset full
assert test ! -e security/image-reports/.trivy-cache
run 1 'high:local critical:local clean:local unfixed:local'
assert test -d security/image-reports/.trivy-cache
assert test -w security/image-reports/.trivy-cache
assert test "$(stat -c '%u' security/image-reports/.trivy-cache)" = "$(id -u)"
assert test -s "$FAKE_STATE/cache-ready"
assert count_is scan 4
assert count_is gate 4
assert count_is db 1
assert count_is java 1
assert grep -q VULNERABLE security/image-reports/summary.tsv
assert grep -q 'CVE-2026-103111' security/image-reports/high_local/report.json
assert grep -q 'CVE-2021-44228' security/image-reports/critical_local/report.json
assert grep -q 'CVE-2026-99999' security/image-reports/unfixed_local/report.json
assert test -s security/image-reports/clean_local/sbom.spdx.json
assert test ! -e "$FAKE_STATE/delays"
assert test ! -e security/image-reports/.audit-lock
assert test -z "$(find security/image-reports -maxdepth 1 -name '.image.*' -print)"
assert test -z "$(grep -E '(^| )build( |$)' "$FAKE_STATE/trace" || true)"

reset full
mkdir -p security/image-reports "$TMP/foreign-cache"
printf 'untouched\n' >"$TMP/foreign-cache/canary"
ln -s "$TMP/foreign-cache" security/image-reports/.trivy-cache
run 2 'clean:local'
assert grep -q 'POLICY: cache must not be a symlink' "$TMP/output"
assert test ! -e "$FAKE_STATE/trace"
assert test ! -e "$FAKE_STATE/cache-ready"
assert test "$(<"$TMP/foreign-cache/canary")" = untouched

reset dedup
run 1 'high:local alias:local high:local'
assert count_is scan 1
assert count_is gate 1
assert count_is sbom 1
assert test -s security/image-reports/alias_local/report.json

reset full
run 0 'clean:local unfixed:local'
assert count_is gate 2
reset full
run 2 'absent:local clean:local'
assert count_is scan 1
assert grep -q POLICY-missing-local-image security/image-reports/summary.tsv
assert test ! -e "$FAKE_STATE/pull"

reset full
id="sha256:$(printf 'c%.0s' {1..64})"
run 0 "$id"
reset platform
run 2 'high:local clean:local'
assert test ! -e "$FAKE_STATE/scan"
reset pull429
run 1 "$ref clean:local"
assert count_is pull 3
assert count_is scan 2
assert test "$(head -1 "$FAKE_STATE/delays")" = 7
reset exhausted
run 2 "$ref clean:local"
assert count_is pull 4
assert count_is scan 1
assert grep -q INCOMPLETE-pull-transient security/image-reports/summary.tsv
for scenario in auth missing signature; do
    reset "$scenario"
    run 2 "$ref clean:local"
    assert count_is pull 1
    assert count_is scan 1
    assert test ! -e "$FAKE_STATE/delays"
done
reset db429
run 0 'clean:local'
assert count_is db 2
assert count_is java 1
reset dbfailed
run 2 'high:local clean:local'
assert count_is db 4
assert count_is java 4
assert test ! -e "$FAKE_STATE/scan"
assert grep -q INCOMPLETE-database security/image-reports/summary.tsv

# Legacy advisory exceptions cannot suppress known exploitation.
reset inheritance
expiry=$(date -u -d "@$(($(date -u +%s) + 7 * 86400))" +%F)
cat >"$IMAGE_SCAN_EXCEPTIONS" <<EOF
- id: CVE-2026-103111
  image: base:test@sha256:$(printf 'e%.0s' {1..64})
  digest: sha256:$(printf 'e%.0s' {1..64})
  package: pcre2
  installed_version: 10.48-r0
  owner: security-test
  reachable: deterministic inherited fixture
  approved: true
  expires: $expiry
EOF
run 1 'high:local'
assert grep -q CVE-2026-103111 security/image-reports/high_local/report.json
sed -i 's/10.48-r0/10.49-r0/' "$IMAGE_SCAN_EXCEPTIONS"
run 1 'high:local'
sed -i '/package:/d; /installed_version:/d' "$IMAGE_SCAN_EXCEPTIONS"
run 1 'high:local'
assert test -s security/image-reports/high_local/report.json
# Content dedup cannot lend one exact reference's approval to another alias.
reset full
cat >"$IMAGE_SCAN_EXCEPTIONS" <<EOF
- id: CVE-2026-103111
  image: high:local@sha256:$(printf 'a%.0s' {1..64})
  digest: sha256:$(printf 'a%.0s' {1..64})
  package: pcre2
  installed_version: 10.48-r0
  owner: security-test
  reachable: exact reference dedup fixture
  approved: true
  expires: $expiry
EOF
run 1 'high:local alias:local'
assert count_is scan 1
assert count_is gate 1
assert grep -Eq 'high:local.*VULNERABLE$' security/image-reports/summary.tsv
assert grep -Eq 'alias:local.*VULNERABLE$' security/image-reports/summary.tsv
# Stale files cannot make a missing ref look scanned in a subsequent run.
reset full
run 0 'clean:local'
printf 'old success\n' >security/image-reports/absent_local_report.json
run 2 'absent:local clean:local'
assert test ! -e security/image-reports/absent_local/report.json
reset advisory
run 0 'high:local'
assert grep -Eq 'high:local.*ADVISORY$' security/image-reports/summary.tsv
assert grep -q CVE-2026-103111 security/image-reports/high_local/report.json
reset exploited-unfixed
run 1 'unfixed:local'
reset kevfailed
run 2 'high:local clean:local'
assert test ! -e "$FAKE_STATE/scan"
# Retry-After budgets and unexpected permanent errors are fail-closed.
# shellcheck source=tools/quality/image-audit/retry.sh
. "$SOURCE/retry.sh"
printf 'HTTP 429; Retry-After: 61\n' >"$TMP/log"
assert test -z "$(retry_delay 1 "$TMP/log" || true)"
printf 'manifest unknown; context deadline exceeded\n' >"$TMP/log"
if retryable_log "$TMP/log"; then exit 1; fi
CHECKS=$((CHECKS + 1))
echo "image-audit regression tests PASSED ($CHECKS checks)"
