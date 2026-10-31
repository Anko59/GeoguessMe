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
        triaged=$(awk '/^#/ || !NF {next} {print}' "$SCRIPT_DIR/not-affected.tsv")
        # A not-affected entry silences only its exact CVE and package, expires
        # within 31 days, and can never override a confirmed deployment exposure.
        jq -er --arg reviewed "$reviewed" --arg triaged "$triaged" '
            def lines($text): $text | split("\n") | map(select(length > 0));
            def rego_set: if length == 0 then "set()" else "{" + (unique | map(tojson) | join(",")) + "}" end;
            def justified: . as $value | any(
                "component_not_present", "vulnerable_code_not_present",
                "vulnerable_code_not_in_execute_path",
                "vulnerable_code_cannot_be_controlled_by_adversary",
                "inline_mitigations_already_exist"; . == $value);
            def expiry: (. + "T00:00:00Z" | fromdateiso8601) + 86400;
            lines($reviewed) as $exposed |
            (lines($triaged) | map(split("\t"))) as $rows |
            if all($rows[];
                length == 6 and
                (.[0] | test("^CVE-[0-9]{4}-[0-9]+$")) and
                (.[1] | test("^[A-Za-z0-9][A-Za-z0-9.+_:-]*$")) and
                .[2] != "" and (.[3] | justified) and
                (.[4] | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")) and
                (.[4] | expiry) > now and (.[4] | expiry) <= now + 32 * 86400 and
                .[5] != "" and
                (.[0] as $id | $exposed | index([$id]) == null)
            ) and ($rows | map(.[0] + "|" + .[1]) | unique | length) == ($rows | length)
            then . else error("not-affected.tsv: malformed, expired, over-long, duplicate or confirmed-exposed entry") end |
            "package trivy\ndefault ignore = false\n" +
            "known_exploited := " + ([.vulnerabilities[].cveID] | rego_set) + "\n" +
            "confirmed_exposed := " + ($exposed | rego_set) + "\n" +
            "not_affected := " + ($rows | map(.[0] + "|" + .[1]) | rego_set) + "\n" +
            "ids[id] { id := input.VulnerabilityID }\nids[id] { id := input.VendorIDs[_] }\n" +
            "exposed { ids[id]; confirmed_exposed[id] }\nexploited { ids[id]; known_exploited[id] }\n" +
            "triaged { not_affected[concat(\"|\", [input.VulnerabilityID, input.PkgName])] }\n" +
            "actionable { exposed }\nactionable {\n  exploited\n  not triaged\n}\n" +
            "ignore {\n  is_string(input.VulnerabilityID)\n  not actionable\n}\n"
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
