#!/usr/bin/env bash
set -euo pipefail
umask 077
fixture=$(mktemp -d)
cleanup() {
    [[ "$fixture" == /tmp/tmp.* && -d "$fixture" ]] || return 1
    rm -rf -- "$fixture"
}
trap cleanup EXIT
mkdir -p "$fixture/bin" "$fixture/frontend/android/app/build/outputs/apk/debug"
touch "$fixture/frontend/android/app/build/outputs/apk/debug/app-debug.apk"
export MOBILE_DEVICE_WORKSPACE="$fixture" HOST_UID="$(id -u)" HOST_GID="$(id -g)"
export CALLS="$fixture/calls" ALL_CALLS="$fixture/all-calls" FIXTURE_MODE=ready CONFIG_MODE=bundled MOBILE_DEVICE_SERIAL=usb-one
export PATH="$fixture/bin:$PATH"
cat >"$fixture/bin/adb" <<'ADB'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$CALLS"
printf '%s\n' "$*" >>"$ALL_CALLS"
if [[ "$*" == 'devices -l' ]]; then
    echo 'List of devices attached'
    case "$FIXTURE_MODE" in
        unavailable) ;;
        unauthorized) echo 'usb-one unauthorized usb:1-1' ;;
        offline) echo 'usb-one offline usb:1-1' ;;
        network) echo 'usb-one device product:fake' ;;
        failure) exit 1 ;;
        *) echo 'usb-one device usb:1-1 product:fake';
            [[ "$FIXTURE_MODE" != multiple ]] || echo 'usb-two device usb:1-2' ;;
    esac
    exit 0
fi
[[ "$1" == -s && "$2" == usb-one ]] || exit 90
shift 2
case "$*" in
    'install -r -t '*) [[ "$FIXTURE_MODE" != conflict ]] ;;
    'shell pidof com.geoguessme.app')
        [[ "$FIXTURE_MODE" != stopped ]] || exit 1
        if [[ "$FIXTURE_MODE" == badpid ]]; then echo 'unsafe pid'; else echo '123 456'; fi ;;
    'shell dumpsys package com.geoguessme.app')
        printf 'private-intent-secret\nversionCode=42\nversionName=1.2\n' ;;
    'shell getprop ro.build.version.release') echo '15' ;;
    'shell getprop ro.build.version.sdk') echo '35' ;;
    'shell dumpsys webviewupdate')
        printf 'private-other-secret\nCurrent WebView package (name, version): (com.google.android.webview, 130.1)\nValid package com.android.chrome (versionName: 130.2)\n' ;;
    'logcat -d --pid=123 -v threadtime'|'logcat -d --pid=456 -v threadtime')
        [[ "$FIXTURE_MODE" != logfailure ]] || exit 1
        echo 'private-app-log' ;;
    'exec-out screencap -p') [[ "$FIXTURE_MODE" != screenfailure ]] || exit 1; printf '\211PNG\r\n' ;;
    *) exit 91 ;;
esac
ADB
cat >"$fixture/bin/unzip" <<'UNZIP'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$CONFIG_MODE" == corrupt ]]; then exit 1; fi
case "${*: -1}" in
    assets/capacitor.config.json)
        case "$CONFIG_MODE" in
            server) echo '{"server":{"url":"https://private.invalid","hostname":"app.geoguessme.com","androidScheme":"https"}}' ;;
            emptyurl) echo '{"server":{"url":"","hostname":"app.geoguessme.com","androidScheme":"https"}}' ;;
            malformed) echo 'invalid json' ;;
            missing) echo '{}' ;;
            *) echo '{"server":{"hostname":"app.geoguessme.com","androidScheme":"https"}}' ;;
        esac ;;
    assets/public/index.html) [[ "$CONFIG_MODE" != noassets ]] || exit 1; echo '<html></html>' ;;
    *) exit 1 ;;
esac
UNZIP
chmod +x "$fixture/bin/adb" "$fixture/bin/unzip"
script=/workspace/tools/mobile/device.sh
run() { bash "$script" "$@" >"$fixture/stdout" 2>"$fixture/stderr"; }
expect_failure() {
    if run "$1"; then
        echo "Expected failure: $1/$FIXTURE_MODE/$CONFIG_MODE" >&2
        exit 1
    fi
}
assert_no_install() { if grep -q 'install ' "$CALLS"; then
    echo 'Unexpected install' >&2
    exit 1
fi; }
for action in install logs; do
    export MOBILE_DEVICE_SERIAL=
    : >"$CALLS"
    expect_failure "$action"
    [[ ! -s "$CALLS" ]]
done
export MOBILE_DEVICE_SERIAL=not-attached
expect_failure install
export MOBILE_DEVICE_SERIAL='bad;serial'
expect_failure install
export MOBILE_DEVICE_SERIAL=usb-one
for mode in unavailable unauthorized offline network failure; do
    export FIXTURE_MODE="$mode"
    : >"$CALLS"
    expect_failure install
    assert_no_install
done
export FIXTURE_MODE=multiple MOBILE_DEVICE_SERIAL=
expect_failure list
export MOBILE_DEVICE_SERIAL=usb-one
run list
run install
# Multiple attached devices are safe only with an explicit selected serial.
grep -q -- '-s usb-one install -r -t' "$CALLS"
export FIXTURE_MODE=ready
for mode in server emptyurl malformed missing corrupt noassets; do
    export CONFIG_MODE="$mode"
    : >"$CALLS"
    expect_failure install
    assert_no_install
done
export CONFIG_MODE=bundled FIXTURE_MODE=conflict
expect_failure install
export FIXTURE_MODE=ready
run install
for mode in stopped badpid; do
    export FIXTURE_MODE="$mode"
    expect_failure logs
done
export FIXTURE_MODE=ready
run logs
run logs
root="$fixture/.local/mobile/device-artifacts"
shopt -s nullglob
snapshots=("$root"/installed-app-*)
[[ ${#snapshots[@]} == 2 ]]
for snapshot in "${snapshots[@]}"; do
    [[ $(stat -c %a "$snapshot") == 700 ]]
    for file in "$snapshot"/*; do [[ $(stat -c %a "$file") == 600 ]]; done
    [[ -s "$snapshot/screenshot.png" ]]
    grep -q private-app-log "$snapshot/logcat-pid-123.txt"
    grep -q private-app-log "$snapshot/logcat-pid-456.txt"
    grep -q versionName=1.2 "$snapshot/app-version.txt"
    grep -q com.android.chrome "$snapshot/webview-versions.txt"
    if grep -Rq 'private-.*-secret' "$snapshot"; then exit 1; fi
done
for mode in logfailure screenfailure; do
    export FIXTURE_MODE="$mode"
    expect_failure logs
done
if grep -Eq 'uninstall|force-stop|pm clear|logcat -c|am start|reverse|start-server' "$ALL_CALLS"; then exit 1; fi
if grep -Eq 'private-app-log|private-intent-secret|private-other-secret|private.invalid' "$fixture/stdout" "$fixture/stderr"; then exit 1; fi
echo 'USB device install/logging contract fixtures passed'
