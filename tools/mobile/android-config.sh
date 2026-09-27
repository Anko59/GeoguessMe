#!/usr/bin/env bash

# Keep the app build on its target SDK while running Maestro on its documented
# supported Android API range (through API 34).
# shellcheck disable=SC2034
ANDROID_COMPILE_API_LEVEL=36
ANDROID_EMULATOR_API_LEVEL=34
ANDROID_AVD_NAME="geoguessme_api_${ANDROID_EMULATOR_API_LEVEL}"
ANDROID_SYSTEM_IMAGE="system-images;android-${ANDROID_EMULATOR_API_LEVEL};google_apis;x86_64"
