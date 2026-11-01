#!/usr/bin/env bash
# Dockerized policy tests: mock only the public feed transport, not jq or Trivy.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf -- "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/reports"
export RISK_FIXTURE="$TMP/catalog.json"
cat >"$TMP/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == *https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json* ]] || exit 2
while [[ "$1" != --output ]]; do shift; done
cp "$RISK_FIXTURE" "$2"
CURL
chmod +x "$TMP/bin/curl"
export PATH="$TMP/bin:$PATH"
cp "$ROOT/tools/quality/image-audit/risk-policy.sh" "$TMP/"
cp "$ROOT/tools/quality/image-audit/blocking-cves.tsv" "$ROOT/tools/quality/image-audit/not-affected.tsv" "$TMP/"
policy="$TMP/risk-policy.sh"
catalog() {
    jq -n --arg date "$1" '{catalogVersion:"fixture",dateReleased:$date,count:1,
        vulnerabilities:[{cveID:"CVE-2021-44228"}]}' >"$RISK_FIXTURE"
}
catalog "$(date -u +%FT%TZ)"
bash "$policy" prepare "$TMP/reports"
test -s "$TMP/reports/kev.sha256"
bash "$policy" emit "$TMP/reports/kev.json" >"$TMP/policy.rego"
grep -q CVE-2021-44228 "$TMP/policy.rego"
# The tracked not-affected register must stay valid and scoped to CVE|package.
bash "$policy" emit "$TMP/reports/kev.json" >"$TMP/policy.rego"
grep -q '^not_affected := {.*"CVE-2026-111497|brotli-libs"' "$TMP/policy.rego"
day() { date -u -d "@$(($(date -u +%s) + $1 * 86400))" +%F; }
triage() {
    printf '# header\n%s\t%s\towner\t%s\t%s\tevidence\n' "$1" "$2" "$3" "$4" >"$TMP/not-affected.tsv"
}
triage CVE-2026-55555 pkg vulnerable_code_not_present "$(day 10)"
bash "$policy" emit "$TMP/reports/kev.json" >"$TMP/policy.rego"
grep -q '"CVE-2026-55555|pkg"' "$TMP/policy.rego"
for invalid in expired over-long justification identifier duplicate exposed columns; do
    case "$invalid" in
        expired) triage CVE-2026-55555 pkg vulnerable_code_not_present "$(day -1)" ;;
        over-long) triage CVE-2026-55555 pkg vulnerable_code_not_present "$(day 40)" ;;
        justification) triage CVE-2026-55555 pkg trust_me "$(day 10)" ;;
        identifier) triage CVE-bad pkg vulnerable_code_not_present "$(day 10)" ;;
        duplicate)
            triage CVE-2026-55555 pkg vulnerable_code_not_present "$(day 10)"
            entry=$(tail -n 1 "$TMP/not-affected.tsv")
            printf '%s\n' "$entry" >>"$TMP/not-affected.tsv"
            ;;
        exposed) triage CVE-2026-12345 pkg vulnerable_code_not_present "$(day 10)" ;;
        columns) printf 'CVE-2026-55555\tpkg\towner\n' >"$TMP/not-affected.tsv" ;;
    esac
    if [ "$invalid" = exposed ]; then
        printf 'CVE-2026-12345\tsecurity-program\tconfirmed exposed regression fixture\n' >>"$TMP/blocking-cves.tsv"
    fi
    if bash "$policy" emit "$TMP/reports/kev.json" >/dev/null 2>&1; then
        echo "FAIL: accepted $invalid not-affected entry" >&2
        exit 1
    fi
    cp "$ROOT/tools/quality/image-audit/blocking-cves.tsv" "$TMP/"
done
cp "$ROOT/tools/quality/image-audit/not-affected.tsv" "$TMP/"
printf 'CVE-2026-12345\tsecurity-program\tconfirmed exposed regression fixture\n' >>"$TMP/blocking-cves.tsv"
bash "$policy" emit "$TMP/reports/kev.json" >"$TMP/policy.rego"
grep -q CVE-2026-12345 "$TMP/policy.rego"
printf 'invalid\tsecurity-program\tinvalid identifier\n' >>"$TMP/blocking-cves.tsv"
if bash "$policy" emit "$TMP/reports/kev.json" >/dev/null 2>&1; then
    echo 'FAIL: accepted malformed deployment blocklist' >&2
    exit 1
fi
for invalid in stale future malformed empty duplicate count; do
    catalog "$(date -u +%FT%TZ)"
    case "$invalid" in
        stale) catalog 2000-01-01T00:00:00Z ;;
        future) catalog 2099-01-01T00:00:00Z ;;
        malformed) printf 'not JSON\n' >"$RISK_FIXTURE" ;;
        empty)
            jq '.count=0 | .vulnerabilities=[]' "$RISK_FIXTURE" >"$TMP/changed"
            mv "$TMP/changed" "$RISK_FIXTURE"
            ;;
        duplicate)
            jq '.count=2 | .vulnerabilities += .vulnerabilities' "$RISK_FIXTURE" >"$TMP/changed"
            mv "$TMP/changed" "$RISK_FIXTURE"
            ;;
        count)
            jq '.count=99' "$RISK_FIXTURE" >"$TMP/changed"
            mv "$TMP/changed" "$RISK_FIXTURE"
            ;;
    esac
    if bash "$policy" prepare "$TMP/reports" >"$TMP/output" 2>&1; then
        echo "FAIL: accepted $invalid exploit feed" >&2
        exit 1
    fi
    test ! -e "$TMP/reports/kev.sha256"
done
echo 'risk-policy freshness/schema/transport/triage tests PASSED'
