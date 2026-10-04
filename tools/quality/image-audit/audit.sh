#!/usr/bin/env bash
# Pure full-scope image audit: no build, mutable-registry fallback or skipped ref.
# IMAGE_AUDIT_REFS: whitespace-separated immutable registry refs, existing local
# explicit tags, or existing sha256:image IDs. Platform defaults to linux/amd64.
# Exit 1: fixed HIGH/CRITICAL findings. Exit 2: policy/incomplete operations (also
# when findings coexist). Raw JSON remains complete; exceptions affect gate only.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
cd "$ROOT"
# shellcheck source=tools/quality/image-audit/retry.sh
. "$SCRIPT_DIR/retry.sh"
PLATFORM=${IMAGE_AUDIT_PLATFORM:-linux/amd64}
: "${GEOGUESSME_TOOLS_PROJECT:?Make must export GEOGUESSME_TOOLS_PROJECT}"
: "${TOOLS_UID:?Make must export TOOLS_UID}"
: "${TOOLS_GID:?Make must export TOOLS_GID}"
REFS_INPUT=${IMAGE_AUDIT_REFS:-}
if ! [[ "$PLATFORM" =~ ^linux/[a-z0-9]+(/v[0-9]+)?$ ]] || [ -z "${REFS_INPUT//[[:space:]]/}" ]; then
    echo 'image-audit: POLICY: explicit nonempty image set and valid platform required' >&2
    exit 2
fi
read -r -a REFS <<<"${REFS_INPUT//$'\n'/ }"
REPORT_ROOT="$ROOT/security/image-reports"
mkdir -p "$REPORT_ROOT"
[ ! -L "$REPORT_ROOT" ] || {
    echo 'image-audit: POLICY: report root must not be a symlink' >&2
    exit 2
}
# Create bind sources as the caller, not Docker's root-owned short-syntax
# fallback. Otherwise a clean non-root runner cannot initialize either database.
CACHE="$REPORT_ROOT/.trivy-cache"
[ ! -L "$CACHE" ] || {
    echo 'image-audit: POLICY: cache must not be a symlink' >&2
    exit 2
}
mkdir -p "$CACHE"
[ -w "$CACHE" ] || {
    echo 'image-audit: INCOMPLETE: cache is not writable' >&2
    exit 2
}
LOCK="$REPORT_ROOT/.audit-lock"
mkdir "$LOCK" 2>/dev/null || {
    echo 'image-audit: INCOMPLETE: another audit owns the database snapshot' >&2
    exit 2
}
ARCHIVE=''
discard_archive() {
    if [ -n "$ARCHIVE" ]; then
        # This exact absolute file is allocated by mktemp below, never a ref.
        case "$ARCHIVE" in "$REPORT_ROOT"/.image.*) rm -f -- "$ARCHIVE" ;; *) return 1 ;; esac
        ARCHIVE=''
    fi
}
cleanup() {
    discard_archive
    rmdir "$LOCK"
}
trap cleanup EXIT
trap 'exit 2' INT TERM
SUMMARY="$REPORT_ROOT/summary.tsv"
[ ! -L "$SUMMARY" ] || {
    echo 'image-audit: POLICY: summary must not be a symlink' >&2
    exit 2
}
printf 'reference\timage_id\tplatform\tresult\n' >"$SUMMARY"
POLICY=0 INCOMPLETE=0 FINDINGS=0
record() {
    local ref=$1 id=$2 status=$3
    printf '%s\t%s\t%s\t%s\n' "$ref" "$id" "$PLATFORM" "$status" >>"$SUMMARY"
    echo "image-audit: $status: $ref"
    case "$status" in
        POLICY*) POLICY=$((POLICY + 1)) ;;
        INCOMPLETE*) INCOMPLETE=$((INCOMPLETE + 1)) ;;
        VULNERABLE) FINDINGS=$((FINDINGS + 1)) ;;
    esac
}
trivy() {
    docker compose -p "$GEOGUESSME_TOOLS_PROJECT" -f deployment/compose.tools.yaml --project-directory . \
        run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" trivy \
        trivy --config /dev/null "$@"
}
# Keep DB updates outside the image loop; scans use this exact snapshot. The
# host lock prevents concurrent audits from replacing it during a scan.
DB_OK=1
if ! run_with_retry 'vulnerability database' "$REPORT_ROOT/db-update.log" trivy image --download-db-only; then
    DB_OK=0
fi
if ! run_with_retry 'Java database' "$REPORT_ROOT/java-db-update.log" trivy image --download-java-db-only; then
    DB_OK=0
fi
if ! trivy version --format json >"$REPORT_ROOT/db-snapshot.json" 2>"$REPORT_ROOT/db-version.log"; then
    DB_OK=0
fi
EXCEPTIONS_OK=1
if ! bash tools/quality/image-scan-exceptions-check.sh; then
    EXCEPTIONS_OK=0
fi
# Content scans are deduplicated by immutable config digest and actual platform.
# Gate results are also keyed by exact generated policy, so aliases cannot widen
# each other's exception authorization.
declare -A CONTENT_DIR CONTENT_RESULT GATE_RESULT REF_SEEN
for ref in "${REFS[@]}"; do
    if [ -n "${REF_SEEN[$ref]:-}" ]; then continue; fi
    REF_SEEN[$ref]=1
    if ! [[ "$ref" =~ ^[A-Za-z0-9][A-Za-z0-9._/:@-]*$ ]]; then
        record "$ref" '-' POLICY-invalid-reference
        continue
    fi
    if [[ "$ref" == *@* ]] && ! [[ "$ref" =~ ^[^@]+@sha256:[0-9a-f]{64}$ ]]; then
        record "$ref" '-' POLICY-invalid-digest
        continue
    fi
    safe=$(printf '%s' "$ref" | tr '/:@' '___')
    dir="$REPORT_ROOT/$safe"
    if [ -L "$dir" ]; then
        record "$ref" '-' POLICY-report-symlink
        continue
    fi
    mkdir -p "$dir"
    # Remove stale success artifacts before attempting this required image.
    for file in report.json sbom.spdx.json gate.txt policy.rego; do
        target="$dir/$file"
        case "$target" in "$REPORT_ROOT/$safe/"*) rm -f -- "$target" ;; *) exit 2 ;; esac
    done
    pinned=0
    [[ "$ref" == *@sha256:* ]] && pinned=1
    if ! docker image inspect "$ref" >"$dir/inspect.json" 2>"$dir/inspect.log"; then
        if ((pinned == 0)); then
            record "$ref" '-' POLICY-missing-local-image
            continue
        fi
        if ! run_with_retry "pull $ref" "$dir/pull.log" docker pull --platform "$PLATFORM" "$ref"; then
            record "$ref" '-' "INCOMPLETE-pull-$RETRY_KIND"
            continue
        fi
    fi
    details=$(docker image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}{{if index . "Variant"}}/{{index . "Variant"}}{{end}}' "$ref" 2>"$dir/inspect.log") || {
        record "$ref" '-' INCOMPLETE-inspect
        continue
    }
    read -r id actual <<<"$details"
    if ! [[ "$id" =~ ^sha256:[0-9a-f]{64}$ ]] || [ "$actual" != "$PLATFORM" ]; then
        record "$ref" "${id:--}" POLICY-image-platform-or-id
        continue
    fi
    printf '%s\n' "$details" >"$dir/resolved.txt"
    exception_ref=$ref
    if ((pinned == 0)) && [[ "$ref" != sha256:* ]]; then exception_ref="$ref@$id"; fi
    policy_ok=$EXCEPTIONS_OK
    if ! bash tools/quality/image-scan-exceptions-check.sh --emit-policy "$exception_ref" "$dir/policy.rego"; then
        policy_ok=0
    fi
    # Validate provenance labels when present, and inherit only unchanged
    # package/version findings from that exact reviewed base digest.
    base=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.base.name"}}|{{index .Config.Labels "org.opencontainers.image.base.digest"}}' "$id") || {
        record "$ref" "$id" INCOMPLETE-base-inspect
        continue
    }
    IFS='|' read -r base_name base_digest <<<"$base"
    [ "$base_name" != '<no value>' ] || base_name=''
    [ "$base_digest" != '<no value>' ] || base_digest=''
    if [ -n "$base_name$base_digest" ]; then
        if ! [[ "$base_name" =~ ^[A-Za-z0-9][A-Za-z0-9._/:-]*$ && "$base_digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
            policy_ok=0
        elif ! bash tools/quality/image-scan-exceptions-check.sh --inherit-policy "$base_name@$base_digest" "$dir/policy.rego"; then
            policy_ok=0
        fi
    fi
    if ((DB_OK == 0)); then
        record "$ref" "$id" INCOMPLETE-database
        continue
    fi
    key="$id/$actual"
    if [ -n "${CONTENT_DIR[$key]:-}" ]; then
        source_dir=${CONTENT_DIR[$key]}
        if [ "$source_dir" != "$dir" ] && [ "${CONTENT_RESULT[$key]}" = OK ]; then
            cp "$source_dir/report.json" "$source_dir/sbom.spdx.json" "$dir/"
        fi
    else
        CONTENT_DIR[$key]=$dir
        CONTENT_RESULT[$key]=INCOMPLETE
        ARCHIVE=$(mktemp "$REPORT_ROOT/.image.XXXXXXXX")
        if docker save "$id" -o "$ARCHIVE"; then
            input="/workspace/${ARCHIVE#"$ROOT/"}"
            if run_with_retry "scan $ref" "$dir/scan.log" trivy image \
                --skip-db-update --skip-java-db-update --scanners vuln \
                --severity HIGH,CRITICAL --exit-code 0 --ignorefile /dev/null \
                --format json --output "/workspace/${dir#"$ROOT/"}/report.json" --input "$input"; then
                if trivy convert --ignorefile /dev/null --format spdx-json \
                    --output "/workspace/${dir#"$ROOT/"}/sbom.spdx.json" \
                    "/workspace/${dir#"$ROOT/"}/report.json" >"$dir/sbom.log" 2>&1 &&
                    [ -s "$dir/report.json" ] && [ -s "$dir/sbom.spdx.json" ]; then
                    CONTENT_RESULT[$key]=OK
                fi
            fi
        fi
    fi
    if [ "${CONTENT_RESULT[$key]}" != OK ]; then
        discard_archive
        record "$ref" "$id" INCOMPLETE-scan
        continue
    fi
    if ((policy_ok == 0)); then
        discard_archive
        record "$ref" "$id" POLICY-exceptions-or-provenance
        continue
    fi
    policy_hash=$(sha256sum "$dir/policy.rego" | cut -d ' ' -f1)
    gate_key="$key/$policy_hash"
    if [ -z "${GATE_RESULT[$gate_key]:-}" ]; then
        # convert lacks --ignore-unfixed in pinned Trivy 0.73. Use its native
        # image filter rather than reimplementing fixed/severity semantics.
        archive_ok=1
        if [ -z "$ARCHIVE" ]; then
            ARCHIVE=$(mktemp "$REPORT_ROOT/.image.XXXXXXXX")
            docker save "$id" -o "$ARCHIVE" || archive_ok=0
        fi
        result='INCOMPLETE-gate'
        if ((archive_ok)); then
            input="/workspace/${ARCHIVE#"$ROOT/"}"
            if run_with_retry "gate $ref" "$dir/gate.log" trivy image \
                --skip-db-update --skip-java-db-update --scanners vuln \
                --severity HIGH,CRITICAL --ignore-unfixed --exit-code 42 \
                --ignorefile /dev/null --ignore-policy "/workspace/${dir#"$ROOT/"}/policy.rego" \
                --format table --output "/workspace/${dir#"$ROOT/"}/gate.txt" --input "$input"; then
                result=OK
            else
                rc=$?
                if ((rc == 42)); then result=VULNERABLE; else result="INCOMPLETE-gate-$RETRY_KIND"; fi
            fi
        fi
        GATE_RESULT[$gate_key]=$result
    fi
    discard_archive
    record "$ref" "$id" "${GATE_RESULT[$gate_key]}"
    [ ! -s "$dir/gate.txt" ] || cat "$dir/gate.txt"
done
cat "$SUMMARY"
echo "image-audit: aggregate: $FINDINGS vulnerable, $POLICY policy failures, $INCOMPLETE incomplete"
((POLICY == 0 && INCOMPLETE == 0)) || exit 2
((FINDINGS == 0)) || exit 1
