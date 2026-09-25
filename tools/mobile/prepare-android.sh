#!/usr/bin/env bash
set -euo pipefail

sdk_root=${ANDROID_SDK_ROOT:?ANDROID_SDK_ROOT is required}
commandline_revision=13114758
commandline_sha256=7ec965280a073311c339e571cd5de778b9975026cfcbe79f2b1cdcb1e15317ee
api_level=36
avd_name=geoguessme_api_36
system_image="system-images;android-${api_level};google_apis;x86_64"

if [[ ! -x "$sdk_root/cmdline-tools/latest/bin/sdkmanager" ]]; then
    archive=$(mktemp)
    trap 'rm -f "$archive"' EXIT
    curl --fail --location --silent --show-error \
        "https://dl.google.com/android/repository/commandlinetools-linux-${commandline_revision}_latest.zip" \
        --output "$archive"
    echo "$commandline_sha256  $archive" | sha256sum --check --strict
    mkdir -p "$sdk_root/cmdline-tools/latest"
    unzip -q "$archive" -d "$sdk_root/cmdline-tools"
    mv "$sdk_root/cmdline-tools/cmdline-tools"/* "$sdk_root/cmdline-tools/latest/"
    rmdir "$sdk_root/cmdline-tools/cmdline-tools"
fi

set +o pipefail
yes | sdkmanager --licenses >/dev/null
license_status=${PIPESTATUS[1]}
set -o pipefail
if ((license_status != 0)); then
    echo "Android SDK license acceptance failed with status $license_status" >&2
    exit "$license_status"
fi
sdkmanager \
    "platform-tools" \
    "emulator" \
    "platforms;android-${api_level}" \
    "build-tools;36.0.0" \
    "$system_image"

mkdir -p /root/.android
if ! avdmanager list avd | grep -Fq "Name: $avd_name"; then
    echo no | avdmanager create avd --force --name "$avd_name" --package "$system_image" --device pixel_6
fi

# Keep the ephemeral test device sparse enough for constrained CI runners. The
# app fixture needs neither a virtual SD card nor a preallocated data image.
avd_config="/root/.android/avd/${avd_name}.avd/config.ini"
sed -i \
    -E \
    -e 's/^disk\.cachePartition[[:space:]]*=.*/disk.cachePartition = no/' \
    -e 's/^disk\.dataPartition\.size[[:space:]]*=.*/disk.dataPartition.size = 1073741824/' \
    -e 's/^sdcard\.size[[:space:]]*=.*/sdcard.size = 64M/' \
    -e 's/^userdata\.useQcow2[[:space:]]*=.*/userdata.useQcow2 = yes/' \
    "$avd_config"

echo "Android SDK and $avd_name are ready."
