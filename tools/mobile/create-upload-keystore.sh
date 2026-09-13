#!/usr/bin/env bash
set -euo pipefail

keystore=${MOBILE_KEYSTORE_PATH:?MOBILE_KEYSTORE_PATH is required}
key_alias=${MOBILE_KEY_ALIAS:?MOBILE_KEY_ALIAS is required}
store_password=${MOBILE_KEYSTORE_PASSWORD:?MOBILE_KEYSTORE_PASSWORD is required}
key_password=${MOBILE_KEY_PASSWORD:?MOBILE_KEY_PASSWORD is required}

if [[ -e "$keystore" ]]; then
    echo "Refusing to overwrite existing keystore: $keystore" >&2
    exit 1
fi
if ((${#store_password} < 6 || ${#key_password} < 6)); then
    echo 'Keystore and key passwords must contain at least six characters.' >&2
    exit 1
fi

umask 077
mkdir -p "$(dirname "$keystore")"
keytool -genkeypair \
    -keystore "$keystore" \
    -storetype JKS \
    -storepass "$store_password" \
    -keypass "$key_password" \
    -alias "$key_alias" \
    -keyalg RSA \
    -keysize 4096 \
    -validity 10000 \
    -dname 'CN=GeoGuessMe Upload'
chmod 600 "$keystore"
chown "${HOST_UID:?HOST_UID is required}:${HOST_GID:?HOST_GID is required}" "$keystore"
echo "Created upload keystore: $keystore"
