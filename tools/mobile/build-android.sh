#!/usr/bin/env bash
set -euo pipefail

cd /workspace/frontend/android
./gradlew --no-daemon --stacktrace assembleDebug
chown -R "${HOST_UID:?HOST_UID is required}:${HOST_GID:?HOST_GID is required}" /workspace/frontend/android
echo "APK: frontend/android/app/build/outputs/apk/debug/app-debug.apk"
