#!/usr/bin/env bash
set -euo pipefail

repo=/workspace
sdk_root=${ANDROID_SDK_ROOT:?ANDROID_SDK_ROOT is required}
artifact_dir="$repo/.local/mobile/artifacts"
apk="$repo/frontend/android/app/build/outputs/apk/debug/app-debug.apk"
avd_name=geoguessme_api_36
app_id=com.geoguessme.app
emulator_log="$artifact_dir/emulator.log"
maestro_log="$artifact_dir/maestro.log"
mkdir -p "$artifact_dir"

if [[ ! -f "$apk" ]]; then
    echo "APK not found at $apk; run make mobile-build first" >&2
    exit 2
fi

diagnostics() {
    timeout 10 adb exec-out screencap -p >"$artifact_dir/failure-screen.png" 2>/dev/null || true
    timeout 10 adb logcat -d -v threadtime >"$artifact_dir/logcat.txt" 2>/dev/null || true
    timeout 10 adb shell dumpsys activity top >"$artifact_dir/activity.txt" 2>/dev/null || true
    timeout 10 adb shell dumpsys package "$app_id" >"$artifact_dir/package.txt" 2>/dev/null || true
    timeout 10 adb shell uiautomator dump /sdcard/window.xml >/dev/null 2>&1 || true
    timeout 10 adb pull /sdcard/window.xml "$artifact_dir/window.xml" >/dev/null 2>&1 || true
    "$sdk_root/emulator/emulator" -accel-check >"$artifact_dir/acceleration.txt" 2>&1 || true
    chown -R "${HOST_UID:-1000}:${HOST_GID:-1000}" "$artifact_dir" || true
}

shutdown() {
    if [[ -n ${location_feed_pid:-} ]]; then
        kill "$location_feed_pid" 2>/dev/null || true
        wait "$location_feed_pid" 2>/dev/null || true
    fi
    timeout 10 adb emu kill >/dev/null 2>&1 || true
    if [[ -n ${emulator_pid:-} ]]; then
        wait "$emulator_pid" 2>/dev/null || true
    fi
}
trap shutdown EXIT INT TERM

acceleration=(-accel off)
if [[ -r /dev/kvm && -w /dev/kvm ]]; then
    acceleration=(-accel on)
fi

"$sdk_root/emulator/emulator" "@$avd_name" \
    -no-window -no-audio -no-boot-anim -no-snapshot -no-cache -wipe-data -qcow2-for-userdata \
    -datadir /emulator-data -data /emulator-data/userdata-qemu.img \
    -gpu swiftshader_indirect "${acceleration[@]}" \
    -camera-front "imagefile:$repo/frontend/public/logo.png" \
    -camera-back "imagefile:$repo/frontend/public/logo.png" \
    >"$emulator_log" 2>&1 &
emulator_pid=$!

adb start-server >/dev/null
deadline=$((SECONDS + 240))
until [[ $(adb get-state 2>/dev/null) == device ]]; do
    if ((SECONDS >= deadline)); then
        echo "Android emulator did not expose adb within 240 seconds" >&2
        diagnostics
        exit 1
    fi
    if ! kill -0 "$emulator_pid" 2>/dev/null; then
        echo "Android emulator exited before adb became available" >&2
        diagnostics
        exit 1
    fi
    sleep 1
done
until [[ $(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r') == 1 ]]; do
    if ((SECONDS >= deadline)); then
        echo "Android emulator did not finish booting within 240 seconds" >&2
        diagnostics
        exit 1
    fi
    if ! kill -0 "$emulator_pid" 2>/dev/null; then
        echo "Android emulator exited before boot completed" >&2
        diagnostics
        exit 1
    fi
    sleep 1
done

adb shell settings put global window_animation_scale 0
adb shell settings put global transition_animation_scale 0
adb shell settings put global animator_duration_scale 0
adb shell cmd location set-location-enabled true
# `geo fix` is an event, not a durable cached position. Keep the virtual GPS
# source active while the journey runs so a later native request is reliable.
(
    while kill -0 "$emulator_pid" 2>/dev/null; do
        adb emu geo fix 2.3522 48.8566 35 12 >/dev/null || exit 0
        sleep 1
    done
) &
location_feed_pid=$!
adb install -r -t "$apk"
adb reverse "tcp:${GEOGUESSME_MOBILE_WEB_PORT:-18081}" "tcp:${GEOGUESSME_MOBILE_WEB_PORT:-18081}"
for permission in \
    android.permission.CAMERA \
    android.permission.RECORD_AUDIO \
    android.permission.ACCESS_COARSE_LOCATION \
    android.permission.ACCESS_FINE_LOCATION; do
    adb shell pm grant "$app_id" "$permission"
done

export MAESTRO_CLI_NO_ANALYTICS=1
if ! maestro test \
    -e "MOBILE_USERNAME=${MOBILE_USERNAME:?MOBILE_USERNAME is required}" \
    -e "MOBILE_PASSWORD=${MOBILE_PASSWORD:?MOBILE_PASSWORD is required}" \
    -e "MOBILE_GROUP_NAME=${MOBILE_GROUP_NAME:?MOBILE_GROUP_NAME is required}" \
    --format junit --output "$artifact_dir/results.xml" \
    "$repo/frontend/mobile-e2e/android.yaml" 2>&1 | tee "$maestro_log"; then
    diagnostics
    exit 1
fi

adb logcat -d -v threadtime '*:E' >"$artifact_dir/logcat-errors.txt" 2>/dev/null || true
chown -R "${HOST_UID:-1000}:${HOST_GID:-1000}" "$artifact_dir"
echo "Maestro Android flow passed. Artifacts: .local/mobile/artifacts"
