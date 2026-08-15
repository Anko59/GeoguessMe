#!/bin/sh
set -eu
set -f

allowed_environment=${1:-}
case "$allowed_environment" in dev | production) ;; *) exit 126 ;; esac

# SSH_ORIGINAL_COMMAND is untrusted. Parse a deliberately tiny protocol and
# reject shell metacharacters through strict per-field validation in deploy.sh
# or through the fixed 2-field verify form below.
# shellcheck disable=SC2086
set -- ${SSH_ORIGINAL_COMMAND:-}
case "${1:-}" in
    deploy)
        [ "$#" -eq 4 ] || {
            printf 'expected: deploy BACKEND_IMAGE WEB_IMAGE REVISION\n' >&2
            exit 126
        }
        [ "$1" = deploy ] || exit 126
        exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4"
        ;;
    verify)
        # Authenticated runtime-integrity check; no credentials or payload are
        # accepted. Execute only the root-owned provisioned verifier; a release
        # directory is writable by the deploy account and must never supply
        # code for this trust check.
        [ "$#" -eq 2 ] || {
            printf 'expected: verify ENVIRONMENT\n' >&2
            exit 126
        }
        [ "$2" = "$allowed_environment" ] || exit 126
        exec "${GEOGUESSME_APP_ROOT:-/opt/geoguessme}/bin/verify-deployment-hashes.sh" "$allowed_environment"
        ;;
    *)
        exit 126
        ;;
esac
