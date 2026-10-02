#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091
source "$repo/tools/mobile/android-config.sh"

[[ $ANDROID_COMPILE_API_LEVEL == 36 ]]
[[ $ANDROID_EMULATOR_API_LEVEL == 34 ]]
[[ $ANDROID_AVD_NAME == "geoguessme_api_${ANDROID_EMULATOR_API_LEVEL}_aosp" ]]
[[ $ANDROID_SYSTEM_IMAGE == "system-images;android-${ANDROID_EMULATOR_API_LEVEL};default;x86_64" ]]
grep -Eq 'targetSdkVersion = 36' "$repo/frontend/android/variables.gradle"
grep -Fq 'source /workspace/tools/mobile/android-config.sh' "$repo/tools/mobile/prepare-android.sh"
# These checks intentionally match literal shell parameter expansions in source.
# shellcheck disable=SC2016
grep -Fq '"platforms;android-${ANDROID_COMPILE_API_LEVEL}"' "$repo/tools/mobile/prepare-android.sh"
# shellcheck disable=SC2016
grep -Fq '"$ANDROID_SYSTEM_IMAGE"' "$repo/tools/mobile/prepare-android.sh"
# shellcheck disable=SC2016
grep -Fq 'source "$repo/tools/mobile/android-config.sh"' "$repo/tools/mobile/run-maestro.sh"
# shellcheck disable=SC2016
grep -Fq '"@$ANDROID_AVD_NAME"' "$repo/tools/mobile/run-maestro.sh"
grep -Fq 'API 36 tools and the Maestro-supported API 34 AOSP AVD' "$repo/docs/mobile.md"
# shellcheck disable=SC2016
# Match the literal variable expansion in the source rather than evaluating it.
grep -Fq 'ANDROID_AVD_NAME="geoguessme_api_${ANDROID_EMULATOR_API_LEVEL}_aosp"' "$repo/tools/mobile/android-config.sh"

echo "Android API build/runtime contract is consistent."
