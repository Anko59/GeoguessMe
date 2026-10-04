#!/usr/bin/env bash
# Deterministic transport fixture. Never used by production audit.
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_STATE/trace"
count() {
    local file="$FAKE_STATE/$1" n=0
    [ ! -f "$file" ] || read -r n <"$file"
    n=$((n + 1))
    printf '%s\n' "$n" >"$file"
    COUNT=$n
}
id_for() {
    case "$1" in
        high:* | alias:* | remote/*) CHAR=a ;;
        critical:*) CHAR=b ;;
        clean:*) CHAR=c ;;
        unfixed:*) CHAR=d ;;
        base:*) CHAR=e ;;
        sha256:*)
            printf '%s\n' "$1"
            return
            ;;
        *) return 1 ;;
    esac
    printf 'sha256:%s\n' "$(printf "$CHAR%.0s" {1..64})"
}
if [ "$1" = image ] && [ "$2" = inspect ]; then
    ref=${!#}
    if [[ "$ref" == remote/* ]] && [ ! -f "$FAKE_STATE/pulled" ]; then exit 1; fi
    id=$(id_for "$ref") || {
        echo 'No such image' >&2
        exit 1
    }
    if [ "${3:-}" != --format ]; then
        printf '[{}]\n'
        exit 0
    fi
    case "$4" in
        '{{.Id}}') echo "$id" ;;
        '{{.Id}} '*)
            # Docker 29 omits Variant entirely for ordinary AMD64 images.
            # Direct field lookup errors; index safely treats absence as empty.
            if [[ "$4" == *'.Variant'* ]]; then
                echo 'template: map has no entry for key Variant' >&2
                exit 1
            fi
            [[ "$4" == *'index . "Variant"'* ]] || exit 9
            platform=linux/amd64
            [ "${FAKE_SCENARIO:-}" != platform ] || platform=linux/arm64
            echo "$id $platform"
            ;;
        *base.name*)
            if [ "${FAKE_SCENARIO:-}" = inheritance ]; then
                printf 'base:test|sha256:%s\n' "$(printf 'e%.0s' {1..64})"
            else printf '<no value>|<no value>\n'; fi
            ;;
        *)
            echo 'unexpected inspect format' >&2
            exit 8
            ;;
    esac
    exit 0
fi
if [ "$1" = pull ]; then
    count pull
    case "${FAKE_SCENARIO:-}" in
        pull429)
            if ((COUNT < 3)); then
                echo 'HTTP 429 Too Many Requests; Retry-After: 7' >&2
                exit 1
            fi
            ;;
        exhausted)
            echo 'HTTP 503 service unavailable' >&2
            exit 1
            ;;
        auth)
            echo 'unauthorized: HTTP 401; i/o timeout' >&2
            exit 1
            ;;
        missing)
            echo 'manifest unknown: HTTP 404' >&2
            exit 1
            ;;
        signature)
            echo 'signature verification failed; HTTP 503' >&2
            exit 1
            ;;
    esac
    touch "$FAKE_STATE/pulled"
    exit 0
fi
if [ "$1" = save ]; then
    printf '%s\n' "$2" >"$4"
    exit 0
fi
if [ "$1" != compose ]; then
    echo 'unexpected Docker command (including any build)' >&2
    exit 9
fi
# Skip Compose options to the actual binary invocation.
while [ "$1" != trivy ]; do shift; done
shift
[ "$1" = trivy ] || exit 9
shift
[ "${1:-}" != --config ] || shift 2
command=$1
shift
args=("$@") output='' input='' policy='' gate=0 db=0 java=0
for ((i = 0; i < ${#args[@]}; i++)); do
    case "${args[$i]}" in
        --download-db-only) db=1 ;;
        --download-java-db-only) java=1 ;;
        --output) output=${args[$((i + 1))]} ;;
        --input) input=${args[$((i + 1))]} ;;
        --ignore-policy) policy=${args[$((i + 1))]} ;;
        --exit-code) [ "${args[$((i + 1))]}" != 42 ] || gate=1 ;;
    esac
done
if ((db || java)); then
    label=db
    ((java == 0)) || label=java
    count "$label"
    if [ "${FAKE_SCENARIO:-}" = db429 ] && [ "$label" = db ] && ((COUNT < 2)); then
        echo 'HTTP 429 Too Many Requests' >&2
        exit 1
    fi
    if [ "${FAKE_SCENARIO:-}" = dbfailed ]; then
        echo 'HTTP 502 Bad Gateway' >&2
        exit 1
    fi
    exit 0
fi
if [ "$command" = version ]; then
    echo '{"Version":"fixture","VulnerabilityDB":{"Version":2}}'
    exit 0
fi
output=${output/#\/workspace/$FAKE_ROOT}
if [ "$command" = convert ]; then
    printf '{"spdxVersion":"SPDX-2.3","packages":[]}\n' >"$output"
    count sbom
    exit 0
fi
[[ " $* " == *' --skip-db-update '* && " $* " == *' --skip-java-db-update '* ]] || exit 9
input=${input/#\/workspace/$FAKE_ROOT}
read -r id <"$input"
if ((gate)); then
    count gate
    printf 'native fixture gate %s\n' "$id" >"$output"
    policy=${policy/#\/workspace/$FAKE_ROOT}
    if [[ "$id" == sha256:b* ]]; then exit 42; fi
    if [[ "$id" == sha256:a* ]]; then
        # The actual pinned-native-policy regression separately checks filtering.
        if grep -Fq 'input.VulnerabilityID == "CVE-2026-103111"' "$policy" &&
            grep -Fq 'input.PkgName == "pcre2"' "$policy" &&
            grep -Fq 'input.InstalledVersion == "10.48-r0"' "$policy"; then exit 0; fi
        exit 42
    fi
    exit 0
fi
count scan
case "$id" in
    sha256:a*) cp "$FAKE_FIXTURES/fixed-high.json" "$output" ;;
    sha256:b*) cp "$FAKE_FIXTURES/fixed-critical.json" "$output" ;;
    *) printf '{"SchemaVersion":2,"ArtifactName":"fixture","ArtifactType":"container_image","Results":[]}\n' >"$output" ;;
esac
if [[ "$id" == sha256:d* ]]; then
    # Unfixed finding remains in full JSON, while the native gate model passes.
    printf '{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2026-99999","Severity":"HIGH","FixedVersion":""}]}]}\n' >"$output"
fi
