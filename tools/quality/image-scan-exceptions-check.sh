#!/usr/bin/env bash
# Validate committed final-image scan exceptions and emit per-image trivy
# ignorefiles for `make audit-images`.
#
# Usage:
#   image-scan-exceptions-check.sh                 # validate only (fail fast)
#   image-scan-exceptions-check.sh --emit REF OUT   # validate, then replace the
#                                                   # ignorefile with REF matches
#   image-scan-exceptions-check.sh --append REF OUT # validate, then append REF
#                                                   # matches to the ignorefile
#
# The exceptions file (IMAGE_SCAN_EXCEPTIONS or
# tools/quality/image-scan-exceptions.yaml) is a YAML list of records:
#
#   - id: CVE-2026-00000
#     image: postgres:15-alpine@sha256:...
#     digest: sha256:...
#     owner: platform@geoguessme.dev
#     reachable: "one-line rationale"
#     approved: true
#     expires: 2026-08-30
#
# Enforcement (F-01): only FIXED High/Critical findings may be excepted; every
# record requires all fields, `approved: true`, a well-formed `sha256:` digest,
# and an `expires` date that is today or later and at most 30 days away.
set -euo pipefail

EXCEPTIONS_FILE="${IMAGE_SCAN_EXCEPTIONS:-tools/quality/image-scan-exceptions.yaml}"

usage() {
    echo "usage: $0 [--emit|--append IMAGE_REF OUTFILE]" >&2
    exit 2
}

MODE=validate
REF=""
OUT=""
case "${1:-}" in
    "") ;;
    --emit | --append)
        [ $# -eq 3 ] || usage
        MODE=${1#--}
        REF=$2
        OUT=$3
        ;;
    *)
        usage
        ;;
esac

[ -f "$EXCEPTIONS_FILE" ] || {
    echo "ERROR: exceptions file not found: $EXCEPTIONS_FILE" >&2
    exit 1
}

date_utc_days_from_today() {
    local days=$1
    if date -u -d "$days days" +%F 2>/dev/null; then
        return
    fi
    if [ "$days" -ge 0 ]; then
        date -u -v+"${days}"d +%F
    else
        date -u -v"${days}"d +%F
    fi
}

# Parse the constrained YAML list with awk. Each record is emitted as one
# tab-separated line in a fixed field order so bash can validate it.
records=$(
    awk '
        /^[[:space:]]*#/ { next }
        /^[[:space:]]*$/ { next }
        /^[[:space:]]*-[[:space:]]*id:[[:space:]]+/ {
            if (rec) emit()
            rec = 1
            id = $0; sub(/^[[:space:]]*-[[:space:]]*id:[[:space:]]*/, "", id)
            sub(/[[:space:]]+$/, "", id)
            image = ""; digest = ""; owner = ""; reachable = ""; approved = ""; expires = ""
            next
        }
        rec && /^[[:space:]]+[a-zA-Z0-9_]+:[[:space:]]*/ {
            key = $0; sub(/^[[:space:]]+/, "", key); sub(/:.*/, "", key)
            val = $0; sub(/^[[:space:]]*[a-zA-Z0-9_]+:[[:space:]]*/, "", val)
            sub(/[[:space:]]+$/, "", val)
            gsub(/^"|"$/, "", val)
            if (key == "image") image = val
            else if (key == "digest") digest = val
            else if (key == "owner") owner = val
            else if (key == "reachable") reachable = val
            else if (key == "approved") approved = val
            else if (key == "expires") expires = val
            else { print "ERROR: unknown key '"key"' in exceptions file" > "/dev/stderr"; bad = 1 }
            next
        }
        rec { print "ERROR: unparsable line: " $0 > "/dev/stderr"; bad = 1 }
        { next }
        END { if (rec) emit(); if (bad) exit 1 }
        function emit() {
            printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, image, digest, owner, reachable, approved, expires
        }
    ' "$EXCEPTIONS_FILE"
) || exit 1

today=$(date -u +%F)
max_expiry=$(date_utc_days_from_today 30)

fail=0
record_count=0

# validate_record: checks one tab-separated record; emits matching trivy
# ignorefile lines to OUT when running in emit mode.
validate_record() {
    local id=$1 image=$2 digest=$3 owner=$4 reachable=$5 approved=$6 expires=$7
    local msg="" image_digest=""

    record_count=$((record_count + 1))

    [ -n "$id" ] || msg="${msg} missing id;"
    [ -n "$image" ] || msg="${msg} missing image;"
    [ -n "$digest" ] || msg="${msg} missing digest;"
    [ -n "$owner" ] || msg="${msg} missing owner;"
    [ -n "$reachable" ] || msg="${msg} missing reachable;"
    [ -n "$approved" ] || msg="${msg} missing approved;"
    [ -n "$expires" ] || msg="${msg} missing expires;"

    [ -n "$msg" ] && {
        echo "ERROR: exception '${id:-<unnamed>}':$msg" >&2
        fail=1
        return
    }

    printf '%s' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
        echo "ERROR: exception '$id' has malformed digest: $digest (expected sha256:<64 hex>)" >&2
        fail=1
    }

    case "$image" in
        *@sha256:*)
            image_digest="sha256:${image##*@sha256:}"
            [ "$image_digest" = "$digest" ] || {
                echo "ERROR: exception '$id' image digest does not match digest field" >&2
                fail=1
            }
            ;;
        *)
            echo "ERROR: exception '$id' image is not digest-pinned: $image" >&2
            fail=1
            ;;
    esac

    [ "$approved" = "true" ] || {
        echo "ERROR: exception '$id' is not approved (approved must be true)" >&2
        fail=1
    }

    case "$expires" in
        [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
        *)
            echo "ERROR: exception '$id' has malformed expires: $expires (expected YYYY-MM-DD)" >&2
            fail=1
            return
            ;;
    esac
    # ISO dates compare lexicographically.
    [ "$expires" \< "$today" ] && {
        echo "ERROR: exception '$id' expires in the past: $expires (today $today)" >&2
        fail=1
    }
    [ "$expires" \> "$max_expiry" ] && {
        echo "ERROR: exception '$id' expires more than 30 days out: $expires (max $max_expiry)" >&2
        fail=1
    }

    if [ "$MODE" = "emit" ] || [ "$MODE" = "append" ]; then
        local ref_name=${REF%%@sha256:*}
        local ref_digest=""
        case "$REF" in
            *@sha256:*) ref_digest="sha256:${REF##*@sha256:}" ;;
        esac
        local match=0
        [ "$image" = "$REF" ] && match=1
        if [ "$match" -eq 0 ] && [ -n "$ref_digest" ]; then
            [ "$digest" = "$ref_digest" ] && [ "$image" = "$ref_name" ] && match=1
        fi
        if [ "$match" -eq 1 ]; then
            printf '# exception %s (owner %s, expires %s)\n%s\n' "$id" "$owner" "$expires" "$id" >>"$OUT"
        fi
    fi
}

if [ "$MODE" = "emit" ]; then
    : >"$OUT"
elif [ "$MODE" = "append" ]; then
    touch "$OUT"
fi

if [ -n "$records" ]; then
    while IFS="$(printf '\t')" read -r id image digest owner reachable approved expires; do
        validate_record "$id" "$image" "$digest" "$owner" "$reachable" "$approved" "$expires"
    done <<<"$records"
fi

[ "$fail" -eq 0 ] || exit 1

echo "image-scan exceptions OK ($record_count records)"
