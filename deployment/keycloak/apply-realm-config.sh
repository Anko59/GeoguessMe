#!/bin/sh
set -eu

: "${KEYCLOAK_CONFIG_SERVER:?KEYCLOAK_CONFIG_SERVER is required}"
: "${KC_BOOTSTRAP_ADMIN_USERNAME:?KC_BOOTSTRAP_ADMIN_USERNAME is required}"
: "${KC_BOOTSTRAP_ADMIN_PASSWORD:?KC_BOOTSTRAP_ADMIN_PASSWORD is required}"
: "${GEOGUESSME_KEYCLOAK_SSL_REQUIRED:?GEOGUESSME_KEYCLOAK_SSL_REQUIRED is required}"
: "${GEOGUESSME_KEYCLOAK_SMTP_HOST:?GEOGUESSME_KEYCLOAK_SMTP_HOST is required}"
: "${GEOGUESSME_KEYCLOAK_SMTP_PORT:?GEOGUESSME_KEYCLOAK_SMTP_PORT is required}"
: "${GEOGUESSME_KEYCLOAK_SMTP_FROM:?GEOGUESSME_KEYCLOAK_SMTP_FROM is required}"
: "${GEOGUESSME_GOOGLE_CLIENT_ID:?GEOGUESSME_GOOGLE_CLIENT_ID is required}"
: "${GEOGUESSME_GOOGLE_CLIENT_SECRET:?GEOGUESSME_GOOGLE_CLIENT_SECRET is required}"
: "${GEOGUESSME_GITHUB_CLIENT_ID:?GEOGUESSME_GITHUB_CLIENT_ID is required}"
: "${GEOGUESSME_GITHUB_CLIENT_SECRET:?GEOGUESSME_GITHUB_CLIENT_SECRET is required}"
: "${GEOGUESSME_APPLE_CLIENT_ID:?GEOGUESSME_APPLE_CLIENT_ID is required}"
: "${GEOGUESSME_APPLE_CLIENT_SECRET:?GEOGUESSME_APPLE_CLIENT_SECRET is required}"
: "${GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET:?GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET is required}"
: "${GEOGUESSME_PRODUCTION_REDIRECT_URL:?GEOGUESSME_PRODUCTION_REDIRECT_URL is required}"
: "${GEOGUESSME_PRODUCTION_ORIGIN:?GEOGUESSME_PRODUCTION_ORIGIN is required}"
: "${GEOGUESSME_PRODUCTION_POST_LOGOUT_URLS:?GEOGUESSME_PRODUCTION_POST_LOGOUT_URLS is required}"
: "${GEOGUESSME_DEV_OIDC_CLIENT_SECRET:?GEOGUESSME_DEV_OIDC_CLIENT_SECRET is required}"
: "${GEOGUESSME_DEV_REDIRECT_URL:?GEOGUESSME_DEV_REDIRECT_URL is required}"
: "${GEOGUESSME_DEV_ORIGIN:?GEOGUESSME_DEV_ORIGIN is required}"
: "${GEOGUESSME_DEV_POST_LOGOUT_URLS:?GEOGUESSME_DEV_POST_LOGOUT_URLS is required}"

kcadm=/opt/keycloak/bin/kcadm.sh
config_file=/tmp/geoguessme-kcadm.config

"$kcadm" config credentials --config "$config_file" \
    --server "$KEYCLOAK_CONFIG_SERVER" --realm master \
    --user "$KC_BOOTSTRAP_ADMIN_USERNAME" --password "$KC_BOOTSTRAP_ADMIN_PASSWORD"

"$kcadm" update realms/geoguessme --config "$config_file" \
    -s "sslRequired=$GEOGUESSME_KEYCLOAK_SSL_REQUIRED" \
    -s loginTheme=geoguessme \
    -s registrationAllowed=true \
    -s registrationEmailAsUsername=true \
    -s loginWithEmailAllowed=true \
    -s duplicateEmailsAllowed=false \
    -s editUsernameAllowed=false \
    -s resetPasswordAllowed=true \
    -s verifyEmail=true \
    -s rememberMe=true \
    -s 'passwordPolicy=length(8)' \
    -s "smtpServer.host=$GEOGUESSME_KEYCLOAK_SMTP_HOST" \
    -s "smtpServer.port=$GEOGUESSME_KEYCLOAK_SMTP_PORT" \
    -s "smtpServer.from=$GEOGUESSME_KEYCLOAK_SMTP_FROM" \
    -s smtpServer.fromDisplayName=GeoGuessMe \
    -s "smtpServer.auth=$GEOGUESSME_KEYCLOAK_SMTP_AUTH" \
    -s "smtpServer.starttls=$GEOGUESSME_KEYCLOAK_SMTP_STARTTLS" \
    -s "smtpServer.ssl=$GEOGUESSME_KEYCLOAK_SMTP_SSL" \
    -s "smtpServer.user=$GEOGUESSME_KEYCLOAK_SMTP_USER" \
    -s "smtpServer.password=$GEOGUESSME_KEYCLOAK_SMTP_PASSWORD"

"$kcadm" update users/profile --config "$config_file" -r geoguessme \
    -f /opt/geoguessme/keycloak/user-profile.json

ensure_required_action() {
    provider_id=$1
    name=$2
    priority=$3

    if ! "$kcadm" get authentication/required-actions --config "$config_file" \
        -r geoguessme --fields alias | grep -Fq "\"alias\" : \"$provider_id\""; then
        "$kcadm" create authentication/register-required-action --config "$config_file" \
            -r geoguessme -s "providerId=$provider_id" -s "name=$name"
    fi

    "$kcadm" update "authentication/required-actions/$provider_id" \
        --config "$config_file" -r geoguessme \
        -s enabled=true -s defaultAction=false -s "priority=$priority"
}

ensure_required_action CONFIGURE_TOTP "Configure OTP" 10
ensure_required_action CONFIGURE_RECOVERY_AUTHN_CODES "Recovery Authentication Codes" 20
ensure_required_action UPDATE_PASSWORD "Update Password" 30
ensure_required_action UPDATE_PROFILE "Update Profile" 40
ensure_required_action VERIFY_EMAIL "Verify Email" 50
ensure_required_action webauthn-register "Webauthn Register" 70
ensure_required_action webauthn-register-passwordless "WebAuthn Register Passwordless" 80
ensure_required_action VERIFY_PROFILE "Verify Profile" 90
ensure_required_action delete_credential "Delete Credential" 100
ensure_required_action UPDATE_EMAIL "Update Email" 110
ensure_required_action idp_link "Linking Identity Provider" 120
ensure_required_action update_user_locale "Update User Locale" 1000
echo "Keycloak optional security actions reconciled"

configure_client() {
    client_id=$1
    client_secret=$2
    redirect_url=$3
    origin=$4
    post_logout_urls=$5
    client_uuid=$("$kcadm" get clients --config "$config_file" -r geoguessme \
        -q "clientId=$client_id" --fields id --format csv --noquotes | head -n 1 | tr -d '\r\n')

    if [ -z "$client_uuid" ]; then
        echo "Keycloak realm is missing client $client_id" >&2
        exit 1
    fi

    "$kcadm" update "clients/$client_uuid" --config "$config_file" -r geoguessme \
        -s "secret=$client_secret" \
        -s serviceAccountsEnabled=true \
        -s "redirectUris=[\"$redirect_url\"]" \
        -s "webOrigins=[\"$origin\"]" \
        -s "attributes={\"pkce.code.challenge.method\":\"S256\",\"post.logout.redirect.uris\":\"$post_logout_urls\"}"
}

configure_client geoguessme-production \
    "$GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET" \
    "$GEOGUESSME_PRODUCTION_REDIRECT_URL" \
    "$GEOGUESSME_PRODUCTION_ORIGIN" \
    "$GEOGUESSME_PRODUCTION_POST_LOGOUT_URLS"
configure_client geoguessme-dev \
    "$GEOGUESSME_DEV_OIDC_CLIENT_SECRET" \
    "$GEOGUESSME_DEV_REDIRECT_URL" \
    "$GEOGUESSME_DEV_ORIGIN" \
    "$GEOGUESSME_DEV_POST_LOGOUT_URLS"

grant_account_deletion_role() {
    client_id=$1
    "$kcadm" add-roles --config "$config_file" -r geoguessme \
        --uusername "service-account-$client_id" \
        --cclientid realm-management --rolename manage-users
}

grant_account_deletion_role geoguessme-production
grant_account_deletion_role geoguessme-dev
echo "Keycloak application clients reconciled"

has_real_credentials() {
    client_id=$1
    client_secret=$2

    case "$client_id:$client_secret" in
        *local-*-placeholder* | *replace-with-* | :* | *:) return 1 ;;
        *) return 0 ;;
    esac
}

configure_identity_provider() {
    alias=$1
    order=$2
    client_id=$3
    client_secret=$4
    enabled=false

    if has_real_credentials "$client_id" "$client_secret"; then
        enabled=true
    fi

    "$kcadm" update "realms/geoguessme/identity-provider/instances/$alias" \
        --config "$config_file" \
        -s "enabled=$enabled" \
        -s hideOnLogin=true \
        -s "config.clientId=$client_id" \
        -s "config.clientSecret=$client_secret" \
        -s "config.guiOrder=$order"
}

configure_identity_provider google 1 "$GEOGUESSME_GOOGLE_CLIENT_ID" "$GEOGUESSME_GOOGLE_CLIENT_SECRET"
configure_identity_provider apple 2 "$GEOGUESSME_APPLE_CLIENT_ID" "$GEOGUESSME_APPLE_CLIENT_SECRET"
configure_identity_provider github 3 "$GEOGUESSME_GITHUB_CLIENT_ID" "$GEOGUESSME_GITHUB_CLIENT_SECRET"
echo "Keycloak social providers reconciled"
