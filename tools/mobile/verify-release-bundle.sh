#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "usage: $0 verify|manifest [AAB_PATH] [MANIFEST_PATH]" >&2
    exit 2
}

mode=${1:-}
case "$mode" in
    verify | manifest) ;;
    *) usage ;;
esac
bundle=${2:-/workspace/frontend/android/app/build/outputs/bundle/release/app-release.aab}
if [[ ! -f "$bundle" ]]; then
    echo "Release bundle not found: $bundle" >&2
    exit 1
fi

repo=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
release_version=$(tr -d '[:space:]' <"$repo/.release-version")
if [[ ! "$release_version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo ".release-version must contain major.minor.patch" >&2
    exit 1
fi
release_major=${BASH_REMATCH[1]}
release_minor=${BASH_REMATCH[2]}
release_patch=${BASH_REMATCH[3]}
expected_version_code=$((10#$release_major * 1000000 + 10#$release_minor * 1000 + 10#$release_patch))

bundletool_jar=${BUNDLETOOL_JAR:-/opt/bundletool/bundletool.jar}
if ! java -jar "$bundletool_jar" validate --bundle "$bundle" >/dev/null; then
    echo "Android App Bundle validation failed: $bundle" >&2
    exit 1
fi
manifest_xml=$(java -jar "$bundletool_jar" dump manifest --bundle "$bundle")

manifest_attribute() {
    local attribute=$1
    awk -v needle="$attribute=\"" '
        {
            offset = index($0, needle)
            if (offset > 0) {
                value = substr($0, offset + length(needle))
                sub(/".*/, "", value)
                print value
                exit
            }
        }
    ' <<<"$manifest_xml"
}

actual_package=$(manifest_attribute package)
actual_version_name=$(manifest_attribute android:versionName)
actual_version_code=$(manifest_attribute android:versionCode)

if [[ "$actual_package" != com.geoguessme.app ]]; then
    echo "Unexpected Android package: ${actual_package:-<missing>}" >&2
    exit 1
fi
if [[ "$actual_version_name" != "$release_version" ]]; then
    echo "AAB version name $actual_version_name does not match $release_version" >&2
    exit 1
fi
if [[ "$actual_version_code" != "$expected_version_code" ]]; then
    echo "AAB version code $actual_version_code does not match $expected_version_code" >&2
    exit 1
fi
if [[ ! "$actual_version_code" =~ ^[0-9]+$ ]]; then
    echo "AAB version code is not numeric: $actual_version_code" >&2
    exit 1
fi

if ! jarsigner -verify "$bundle" >/dev/null 2>&1; then
    echo "AAB signature verification failed: $bundle" >&2
    exit 1
fi

certificate_sha256=$(keytool -printcert -jarfile "$bundle" |
    sed -n 's/^[[:space:]]*SHA256:[[:space:]]*//p' |
    awk 'NR == 1 { gsub(/:/, ""); print tolower($0); exit }')
if [[ ! "$certificate_sha256" =~ ^[0-9a-f]{64}$ ]]; then
    echo "Could not extract a SHA-256 upload certificate fingerprint" >&2
    exit 1
fi

expected_certificate=${MOBILE_EXPECTED_UPLOAD_CERT_SHA256:-}
if [[ -n "$expected_certificate" ]]; then
    expected_certificate=$(tr -d ':[:space:]' <<<"$expected_certificate" | tr '[:upper:]' '[:lower:]')
    if [[ ! "$expected_certificate" =~ ^[0-9a-f]{64}$ ]]; then
        echo "MOBILE_EXPECTED_UPLOAD_CERT_SHA256 is not a SHA-256 fingerprint" >&2
        exit 1
    fi
    if [[ "$certificate_sha256" != "$expected_certificate" ]]; then
        echo "AAB upload certificate does not match the configured fingerprint" >&2
        exit 1
    fi
elif [[ "${MOBILE_REQUIRE_EXPECTED_CERT:-false}" == true ]]; then
    echo "MOBILE_EXPECTED_UPLOAD_CERT_SHA256 is required" >&2
    exit 1
fi

aab_sha256=$(sha256sum "$bundle" | awk '{print $1}')
if [[ ! "$aab_sha256" =~ ^[0-9a-f]{64}$ ]]; then
    echo "Could not calculate the AAB SHA-256" >&2
    exit 1
fi

if [[ "$mode" == verify ]]; then
    printf 'AAB verified\npackage=%s\nversion_name=%s\nversion_code=%s\naab_sha256=%s\nupload_certificate_sha256=%s\n' \
        "$actual_package" "$actual_version_name" "$actual_version_code" "$aab_sha256" "$certificate_sha256"
    exit 0
fi

source_sha=${MOBILE_SOURCE_SHA:-unknown}
source_tree=${MOBILE_SOURCE_TREE:-unknown}
if [[ "${MOBILE_REQUIRE_PROVENANCE:-false}" == true && ("$source_sha" == unknown || "$source_tree" == unknown) ]]; then
    echo "MOBILE_SOURCE_SHA and MOBILE_SOURCE_TREE are required" >&2
    exit 1
fi

manifest_json=$(jq -n \
    --arg source_sha "$source_sha" \
    --arg source_tree "$source_tree" \
    --arg package "$actual_package" \
    --arg version_name "$actual_version_name" \
    --arg version_code "$actual_version_code" \
    --arg aab_sha256 "$aab_sha256" \
    --arg upload_certificate_sha256 "$certificate_sha256" \
    --arg workflow_run "${GITHUB_RUN_ID:-local}" \
    '{source_sha: $source_sha,
      source_tree: $source_tree,
      package_name: $package,
      version_name: $version_name,
      version_code: ($version_code | tonumber),
      aab_sha256: $aab_sha256,
      upload_certificate_sha256: $upload_certificate_sha256,
      workflow_run: $workflow_run}')

manifest_path=${3:-}
if [[ -n "$manifest_path" ]]; then
    mkdir -p "$(dirname -- "$manifest_path")"
    printf '%s\n' "$manifest_json" >"$manifest_path"
    printf 'manifest=%s\n' "$manifest_path"
else
    printf '%s\n' "$manifest_json"
fi
