#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
SCRIPT="$ROOT/tools/mobile/verify-release-bundle.sh"
release_version=$(tr -d '[:space:]' <"$ROOT/.release-version")
if [[ ! "$release_version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
    echo ".release-version must contain MAJOR.MINOR.PATCH" >&2
    exit 1
fi
release_major=${BASH_REMATCH[1]}
release_minor=${BASH_REMATCH[2]}
release_patch=${BASH_REMATCH[3]}
expected_version_code=$((10#$release_major * 1000000 + 10#$release_minor * 1000 + 10#$release_patch))
tmpdir=$(mktemp -d)
trap 'rm -rf -- "$tmpdir"' EXIT

fakebin="$tmpdir/bin"
mkdir -p "$fakebin"
printf 'test AAB\n' >"$tmpdir/app-release.aab"
printf 'bundletool fixture\n' >"$tmpdir/bundletool.jar"

cat >"$fakebin/java" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_INVALID:-false}" == true ]]; then
    exit 1
fi
printf '<manifest package="com.geoguessme.app" android:versionCode="%s" android:versionName="%s">\n' \
    "${MOBILE_TEST_VERSION_CODE:?}" "${MOBILE_TEST_VERSION_NAME:?}"
printf '</manifest>\n'
EOF
cat >"$fakebin/jarsigner" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_UNSIGNED:-false}" == true ]]; then
    exit 1
fi
EOF
cat >"$fakebin/keytool" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'Certificate fingerprints:\n'
printf '         SHA256: 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF\n'
EOF
chmod 0555 "$fakebin/java" "$fakebin/jarsigner" "$fakebin/keytool"

base_env=(
    PATH="$fakebin:$PATH"
    BUNDLETOOL_JAR="$tmpdir/bundletool.jar"
    MOBILE_TEST_VERSION_NAME="$release_version"
    MOBILE_TEST_VERSION_CODE="$expected_version_code"
)

assert_contains() {
    local name=$1
    local haystack=$2
    local needle=$3
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "FAIL: $name: missing '$needle'" >&2
        exit 1
    fi
    echo "PASS: $name"
}

assert_failure() {
    local name=$1
    shift
    if "$@" >/dev/null 2>&1; then
        echo "FAIL: $name: command unexpectedly succeeded" >&2
        exit 1
    fi
    echo "PASS: $name"
}

verified=$(env "${base_env[@]}" "$SCRIPT" verify "$tmpdir/app-release.aab")
assert_contains "valid bundle is verified" "$verified" 'AAB verified'
assert_contains "package is reported" "$verified" 'package=com.geoguessme.app'
assert_contains "version code is reported" "$verified" "version_code=$expected_version_code"
assert_contains "certificate is normalized" "$verified" 'upload_certificate_sha256=00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff'

manifest_output="$tmpdir/manifest.json"
manifest=$(env "${base_env[@]}" MOBILE_SOURCE_SHA=abc123 MOBILE_SOURCE_TREE=tree456 \
    MOBILE_REQUIRE_PROVENANCE=true GITHUB_RUN_ID=42 "$SCRIPT" manifest "$tmpdir/app-release.aab" "$manifest_output")
assert_contains "manifest path is reported" "$manifest" "manifest=$manifest_output"
jq -e --arg version_name "$release_version" --argjson version_code "$expected_version_code" '
    .source_sha == "abc123" and
    .source_tree == "tree456" and
    .package_name == "com.geoguessme.app" and
    .version_name == $version_name and
    .version_code == $version_code and
    (.aab_sha256 | test("^[0-9a-f]{64}$")) and
    .upload_certificate_sha256 == "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff" and
    .workflow_run == "42"
' "$manifest_output" >/dev/null
echo "PASS: manifest contains verified provenance"

assert_failure "unsigned bundle is rejected" env "${base_env[@]}" FAKE_UNSIGNED=true "$SCRIPT" verify "$tmpdir/app-release.aab"
assert_failure "invalid bundle is rejected" env "${base_env[@]}" FAKE_INVALID=true "$SCRIPT" verify "$tmpdir/app-release.aab"
assert_failure "wrong certificate is rejected" env "${base_env[@]}" \
    MOBILE_EXPECTED_UPLOAD_CERT_SHA256=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff \
    "$SCRIPT" verify "$tmpdir/app-release.aab"
assert_failure "missing provenance is rejected" env "${base_env[@]}" \
    MOBILE_REQUIRE_PROVENANCE=true "$SCRIPT" manifest "$tmpdir/app-release.aab"

echo "mobile release bundle contract tests PASSED"
