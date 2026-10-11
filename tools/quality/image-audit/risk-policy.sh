#!/usr/bin/env bash
# Run in the Dockerized security helper. Never accept a caller-selected feed.
set -euo pipefail
[[ $# == 2 ]] || {
    echo 'risk-policy: expected prepare|emit|classify PATH' >&2
    exit 2
}
mode=$1 path=$2
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
case "$mode" in
    prepare)
        mkdir -p "$path"
        rm -f -- "$path/kev.json" "$path/kev.sha256"
        curl --proto '=https' --proto-redir '=https' --location --fail --silent --show-error \
            --connect-timeout 10 --max-time 45 \
            https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json \
            --output "$path/kev.json"
        jq -e '
            (.dateReleased | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) as $released |
            (.catalogVersion | type == "string") and
            (.dateReleased | type == "string") and
            $released <= (now + 86400) and
            $released >= (now - 30 * 86400) and
            (.vulnerabilities | type == "array" and length > 0) and
            .count == (.vulnerabilities | length) and
            all(.vulnerabilities[]; .cveID | type == "string" and test("^CVE-[0-9]{4}-[0-9]+$")) and
            ([.vulnerabilities[].cveID] | unique | length) == .count
        ' "$path/kev.json" >/dev/null
        sha256sum "$path/kev.json" >"$path/kev.sha256"
        ;;
    emit)
        reviewed=$(awk -F '\t' '
            /^#/ || !NF {next}
            NF != 3 || $1 !~ /^CVE-[0-9][0-9][0-9][0-9]-[0-9]+$/ || $2 == "" || $3 == "" || seen[$1]++ {bad=1}
            {print $1}
            END {exit bad}
        ' "$SCRIPT_DIR/blocking-cves.tsv")
        jq -er --arg reviewed "$reviewed" '
            "package trivy\ndefault ignore = false\nknown_exploited := {" +
            ([.vulnerabilities[].cveID, ($reviewed | split("\n")[] | select(length > 0))] | unique | map(tojson) | join(",")) +
            "}\nactionable { known_exploited[input.VulnerabilityID] }\nactionable {\n  id := input.VendorIDs[_]\n  known_exploited[id]\n}\nignore {\n  is_string(input.VulnerabilityID)\n  not actionable\n}\n"
        ' "$path"
        ;;
    classify)
        jq -er '
            if ([.Results[]?.Vulnerabilities[]?] | length) > 0 then "ADVISORY" else "OK" end
        ' "$path"
        ;;
    *)
        echo 'risk-policy: unknown mode' >&2
        exit 2
        ;;
esac
