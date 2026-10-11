#!/usr/bin/env bash
# USB-only operator interface; never starts, stops, clears, or uninstalls apps.
set -euo pipefail
umask 077

repo=${MOBILE_DEVICE_WORKSPACE:-/workspace}
app_id=com.geoguessme.app
apk="$repo/frontend/android/app/build/outputs/apk/debug/app-debug.apk"
serial=${MOBILE_DEVICE_SERIAL:-}
action=${1:-}
fail() {
    echo "$*" >&2
    exit 2
}
[[ "$action" == list || "$action" == install || "$action" == logs ]] || fail 'Expected list, install, or logs'
command -v adb >/dev/null || fail 'Android platform-tools unavailable; run make mobile-prepare'
if [[ "$action" != list && -z "$serial" ]]; then
    fail 'Export MOBILE_DEVICE_SERIAL to explicitly select a USB device; run make mobile-device-list'
fi
[[ -z "$serial" || "$serial" =~ ^[a-zA-Z0-9._-]+$ ]] || fail 'Invalid USB device serial'
# The Compose service has an isolated network namespace: do not connect to a
# host/emulator adb server, including one inherited through the environment.
unset ADB_SERVER_SOCKET ANDROID_ADB_SERVER_PORT ANDROID_SERIAL
export ADB_SERVER_PORT=5037
if ! devices=$(timeout 15 adb devices -l 2>/dev/null); then
    fail 'Cannot enumerate USB devices; check Linux USB access and Android USB debugging'
fi
rows=$(printf '%s\n' "$devices" | awk 'NR > 1 && NF >= 2 {print}')
if [[ "$action" == list ]]; then
    printf '%s\n' "$devices"
    [[ -n "$rows" ]] || fail 'No devices available; connect and authorize a Linux USB device'
    if [[ -z "$serial" ]]; then
        count=$(printf '%s\n' "$rows" | awk 'END {print NR}')
        [[ "$count" == 1 ]] || fail 'Multiple devices available; export MOBILE_DEVICE_SERIAL explicitly'
        serial=$(printf '%s\n' "$rows" | awk '{print $1}')
    fi
fi
row=$(printf '%s\n' "$rows" | awk -v serial="$serial" '$1 == serial {print}')
[[ -n "$row" ]] || fail 'Selected device is unavailable'
state=$(printf '%s\n' "$row" | awk '{print $2}')
[[ "$state" == device ]] || fail 'Selected device is not authorized/online; authorize USB debugging on the device'
[[ " $row " == *' usb:'* ]] || fail 'Selected device is not a Linux USB transport'
[[ "$action" != list ]] || exit 0
adb_selected() { timeout 30 adb -s "$serial" "$@"; }

if [[ "$action" == install ]]; then
    [[ -f "$apk" ]] || fail 'Built APK missing; run make mobile-build first'
    # Validate the actual packaged configuration, not a potentially stale source
    # config. Reject even an empty server.url key and missing packaged assets.
    if ! config=$(unzip -p "$apk" assets/capacitor.config.json 2>/dev/null) ||
        ! printf '%s' "$config" | jq -e 'type == "object" and (.server | type == "object") and
            (.server | has("url") | not) and .server.hostname == "app.geoguessme.com" and
            .server.androidScheme == "https"' >/dev/null 2>&1 ||
        ! unzip -p "$apk" assets/public/index.html >/dev/null 2>&1; then
        fail 'Install requires a bundled production-configured APK without server.url; run make mobile-build'
    fi
    # Do not grant permissions or try to fix signing conflicts by removing a
    # Play-installed app. adb diagnostics can contain arbitrary app strings.
    if ! timeout 120 adb -s "$serial" install -r -t "$apk" >/dev/null 2>&1; then
        fail 'APK install failed; existing app/data retained (a Play signature mismatch requires a separate test device)'
    fi
    echo 'Bundled debug APK installed; app was not launched. Existing Play logs should be captured before install.'
    exit 0
fi

if ! pids=$(adb_selected shell pidof "$app_id" 2>/dev/null); then
    fail 'App is not running; open the installed app manually, reproduce, then run make mobile-device-logs'
fi
pids=${pids//$'\r'/}
[[ "$pids" =~ ^[0-9]+([[:space:]][0-9]+)*$ ]] || fail 'Cannot determine app PID safely'
artifact_root="$repo/.local/mobile/device-artifacts"
[[ ! -L "$artifact_root" ]] || fail 'Artifact directory must not be a symbolic link'
mkdir -p "$artifact_root"
chmod 700 "$artifact_root"
artifact_dir=$(mktemp -d "$artifact_root/installed-app-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")
finish() {
    chmod -R go-rwx "$artifact_dir"
    chown -R "${HOST_UID:?HOST_UID is required}:${HOST_GID:?HOST_GID is required}" "$artifact_dir"
    chown "${HOST_UID}:${HOST_GID}" "$artifact_root"
}
trap finish EXIT
# Only version metadata is retained; a full package dump can expose intent data.
adb_selected shell dumpsys package "$app_id" | awk '/versionCode=|versionName=/ {print}' >"$artifact_dir/app-version.txt"
[[ -s "$artifact_dir/app-version.txt" ]] || fail 'Installed app version unavailable; partial private artifacts retained'
adb_selected shell getprop ro.build.version.release >"$artifact_dir/android-release.txt"
adb_selected shell getprop ro.build.version.sdk >"$artifact_dir/android-sdk.txt"
adb_selected shell dumpsys webviewupdate | awk '/Current WebView package|Valid package|Invalid package/ {print}' >"$artifact_dir/webview-versions.txt"
# Keep snapshots separate from emulator/debug artifacts; never clear logcat.
for pid in $pids; do
    adb_selected logcat -d --pid="$pid" -v threadtime >"$artifact_dir/logcat-pid-$pid.txt" 2>"$artifact_dir/logcat-error.txt"
done
adb_selected exec-out screencap -p >"$artifact_dir/screenshot.png" 2>"$artifact_dir/screenshot-error.txt"
[[ -s "$artifact_dir/screenshot.png" ]] || fail 'Device screenshot was empty; partial private artifacts retained'
printf 'Private installed-app snapshot: %s\n' "${artifact_dir#"$repo/"}"
