#!/usr/bin/env bash
# Validate reviewed image exceptions. No name-only or wildcard digest matching.
# --emit/--append emit legacy IDs for unscoped exact-final records only.
# --emit-policy REF OUT creates native Trivy Rego for the exact final image.
# --inherit-policy BASE OUT appends ONLY exact package/version-scoped base rules.
# package + installed_version are optional together for exact-final records and
# mandatory for inheritance. Unchanged base provenance does not authorize a CVE
# exemption for a replacement package or missing package metadata.
set -euo pipefail

EXCEPTIONS_INPUT="${IMAGE_SCAN_EXCEPTIONS:-tools/quality/image-scan-exceptions.yaml tools/quality/image-scan-exceptions-keycloak.yaml tools/quality/image-scan-exceptions-oauth2-proxy.yaml tools/quality/image-scan-exceptions-cloudflared.yaml tools/quality/image-scan-exceptions-sops.yaml}"
read -r -a EXCEPTION_FILES <<<"$EXCEPTIONS_INPUT"
MODE=validate REF='' OUT=''
case "${1:-}" in
    '') [ $# -eq 0 ] || exit 2 ;;
    --emit | --append | --emit-policy | --inherit-policy)
        [ $# -eq 3 ] || {
            echo 'usage: checker [--emit|--append|--emit-policy|--inherit-policy REF OUT]' >&2
            exit 2
        }
        MODE=${1#--} REF=$2 OUT=$3
        ;;
    *)
        echo 'ERROR: unknown exception-checker mode' >&2
        exit 2
        ;;
esac
for file in "${EXCEPTION_FILES[@]}"; do
    [ -f "$file" ] || {
        echo "ERROR: exceptions file not found: $file" >&2
        exit 1
    }
done

# A constrained parser, not general YAML. Unknown/duplicate fields and content
# outside a record fail closed. Pipes are forbidden because they delimit output.
records=$(awk '
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    /^[[:space:]]*-[[:space:]]*id:[[:space:]]+/ {
        if (rec) emit()
        rec = 1; delete f
        val = $0; sub(/^[[:space:]]*-[[:space:]]*id:[[:space:]]*/, "", val)
        sub(/[[:space:]]+$/, "", val); f["id"] = val; next
    }
    rec && /^[[:space:]]+[a-zA-Z0-9_]+:[[:space:]]*/ {
        key = $0; sub(/^[[:space:]]+/, "", key); sub(/:.*/, "", key)
        val = $0; sub(/^[[:space:]]*[a-zA-Z0-9_]+:[[:space:]]*/, "", val)
        sub(/[[:space:]]+$/, "", val); gsub(/^"|"$/, "", val)
        if (key !~ /^(image|digest|owner|reachable|approved|expires|package|installed_version)$/ || key in f) {
            print "ERROR: unknown or duplicate key " key > "/dev/stderr"; bad = 1
        }
        f[key] = val; next
    }
    { print "ERROR: unparsable line: " $0 > "/dev/stderr"; bad = 1 }
    END { if (rec) emit(); if (bad) exit 1 }
    function emit(    i, keys, out) {
        split("id image digest owner reachable approved expires package installed_version", keys, " ")
        if (("package" in f || "installed_version" in f) && (f["package"] == "" || f["installed_version"] == "")) {
            print "ERROR: package and installed_version must both be nonempty" > "/dev/stderr"; bad = 1
        }
        out = ""
        for (i = 1; i <= 9; i++) {
            if (f[keys[i]] ~ /[|\t\r]/) { print "ERROR: invalid field separator" > "/dev/stderr"; bad = 1 }
            out = out (i == 1 ? "" : "|") f[keys[i]]
        }
        print out
    }
' "${EXCEPTION_FILES[@]}") || exit 1

today=$(date -u +%F)
expiry_epoch=$(($(date -u +%s) + 30 * 86400))
max_expiry=$(date -u -d "@$expiry_epoch" +%F 2>/dev/null || date -u -r "$expiry_epoch" +%F)
fail=0 record_count=0
REF_NAME=${REF%%@sha256:*} REF_DIGEST=''
case "$REF" in
    *@sha256:*) REF_DIGEST="sha256:${REF##*@sha256:}" ;;
    sha256:*) REF_DIGEST=$REF ;;
    '') ;;
    *)
        # Legacy local-tag callers must verify the actual image ID. audit.sh
        # freezes tags by ID and supplies REF@ID, avoiding a tag-change race.
        REF_DIGEST=$(docker image inspect --format '{{.Id}}' "$REF" 2>/dev/null || true)
        ;;
esac
if [ "$MODE" != validate ] && ! [[ "$REF_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    echo 'ERROR: emitted exceptions require an exact digest or existing local image' >&2
    exit 1
fi
case "$MODE" in
    emit) : >"$OUT" ;;
    append) touch "$OUT" ;;
    emit-policy) printf 'package trivy\nimport rego.v1\ndefault ignore := false\n' >"$OUT" ;;
    inherit-policy)
        [ -s "$OUT" ] || {
            echo 'ERROR: inherited policy requires an existing final-image policy' >&2
            exit 1
        }
        ;;
esac

validate_record() {
    local id=$1 image=$2 digest=$3 owner=$4 reachable=$5 approved=$6 expires=$7 package=$8 version=$9
    local msg='' calendar match=0
    record_count=$((record_count + 1))
    [ -n "$id" ] || msg+=' missing id;'
    [ -n "$image" ] || msg+=' missing image;'
    [ -n "$digest" ] || msg+=' missing digest;'
    [ -n "$owner" ] || msg+=' missing owner;'
    [ -n "$reachable" ] || msg+=' missing reachable;'
    [ -n "$approved" ] || msg+=' missing approved;'
    [ -n "$expires" ] || msg+=' missing expires;'
    if [ -n "$msg" ]; then
        echo "ERROR: exception '${id:-<unnamed>}':$msg" >&2
        fail=1
        return
    fi
    if ! [[ "$id" =~ ^(CVE-[0-9]{4}-[0-9]+|GHSA-[a-z0-9]+-[a-z0-9]+-[a-z0-9]+)$ ]]; then
        echo "ERROR: exception '$id' has malformed id" >&2
        fail=1
    fi
    if ! [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        echo "ERROR: exception '$id' has malformed digest" >&2
        fail=1
    fi
    case "$image" in
        *@sha256:*)
            if [ "sha256:${image##*@sha256:}" != "$digest" ]; then
                echo "ERROR: exception '$id' image digest does not match digest field" >&2
                fail=1
            fi
            ;;
        *)
            echo "ERROR: exception '$id' image is not digest-pinned" >&2
            fail=1
            ;;
    esac
    if [ "$approved" != true ]; then
        echo "ERROR: exception '$id' is not approved (approved must be true)" >&2
        fail=1
    fi
    if ! [[ "$expires" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        echo "ERROR: exception '$id' has malformed expires" >&2
        fail=1
        return
    fi
    calendar=$(date -u -d "$expires" +%F 2>/dev/null || date -j -u -f '%Y-%m-%d' "$expires" +%F 2>/dev/null || true)
    if [ "$calendar" != "$expires" ]; then
        echo "ERROR: exception '$id' has invalid calendar expiry" >&2
        fail=1
    fi
    if [[ "$expires" < "$today" ]]; then
        echo "ERROR: exception '$id' expires in the past: $expires (today $today)" >&2
        fail=1
    fi
    if [[ "$expires" > "$max_expiry" ]]; then
        echo "ERROR: exception '$id' expires more than 30 days out: $expires (max $max_expiry)" >&2
        fail=1
    fi
    if [ -n "$package$version" ]; then
        if ! [[ "$package" =~ ^[A-Za-z0-9_./:@+~-]+$ && "$version" =~ ^[A-Za-z0-9_.:+~-]+$ ]]; then
            echo "ERROR: exception '$id' requires safe nonempty package and installed_version together" >&2
            fail=1
        fi
    fi
    [ "$MODE" != validate ] || return 0
    [ "${image%%@sha256:*}" = "$REF_NAME" ] && [ "$digest" = "$REF_DIGEST" ] && match=1
    [ "$match" -eq 1 ] || return 0
    case "$MODE" in
        emit | append)
            if [ -n "$package$version" ]; then
                echo "ERROR: exception '$id' requires native scoped policy, not legacy ID emission" >&2
                fail=1
                return
            fi
            printf '# exception %s (owner %s, expires %s)\n%s\n' "$id" "$owner" "$expires" "$id" >>"$OUT"
            ;;
        emit-policy | inherit-policy)
            if [ "$MODE" = inherit-policy ] && [ -z "$package$version" ]; then
                echo "ERROR: exception '$id' cannot be inherited without package and installed_version" >&2
                fail=1
                return
            fi
            # Input strings are constrained above; these are native Trivy
            # predicate rules, not a replacement vulnerability policy engine.
            printf 'ignore if {\n  input.VulnerabilityID == "%s"\n' "$id" >>"$OUT"
            if [ -n "$package$version" ]; then
                printf '  input.PkgName == "%s"\n  input.InstalledVersion == "%s"\n' "$package" "$version" >>"$OUT"
            fi
            printf '}\n' >>"$OUT"
            ;;
    esac
}

if [ -n "$records" ]; then
    while IFS='|' read -r id image digest owner reachable approved expires package version; do
        validate_record "$id" "$image" "$digest" "$owner" "$reachable" "$approved" "$expires" "$package" "$version"
    done <<<"$records"
fi
[ "$fail" -eq 0 ] || exit 1
echo "image-scan exceptions OK ($record_count records)"
