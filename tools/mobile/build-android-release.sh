#!/usr/bin/env bash
set -euo pipefail

keystore=${MOBILE_KEYSTORE_PATH:?MOBILE_KEYSTORE_PATH is required}
if [[ ! -f "$keystore" ]]; then
    echo "Release keystore not found: $keystore" >&2
    exit 1
fi

cd /workspace/frontend/android
./gradlew --no-daemon --stacktrace bundleRelease
chown -R "${HOST_UID:?HOST_UID is required}:${HOST_GID:?HOST_GID is required}" /workspace/frontend/android
echo "AAB: frontend/android/app/build/outputs/bundle/release/app-release.aab"
