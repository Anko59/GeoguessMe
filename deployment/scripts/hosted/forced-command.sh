#!/bin/sh
set -eu
set -f

allowed_environment=${1:-}
case "$allowed_environment" in dev | production) ;; *) exit 126 ;; esac

# SSH_ORIGINAL_COMMAND is untrusted. Parse a deliberately tiny protocol and
# reject shell metacharacters through strict image/revision validation in the
# deployment scripts or through the fixed 2-field verify form below.
# shellcheck disable=SC2086
set -- ${SSH_ORIGINAL_COMMAND:-}
case "${1:-}" in
    deploy)
        [ "$1" = deploy ] || exit 126
        case "$allowed_environment:$#" in
            dev:4) exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4" ;;
            dev:5) exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4" "$5" ;;
            production:5) exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4" "$5" ;;
            production:6) exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4" "$5" "$6" ;;
            *)
                printf 'expected: dev deploy BACKEND_IMAGE WEB_IMAGE [SOPS_IMAGE] REVISION; production deploy BACKEND_IMAGE WEB_IMAGE KEYCLOAK_IMAGE [SOPS_IMAGE] REVISION\n' >&2
                exit 126
                ;;
        esac
        ;;
    watch)
        [ "$#" -eq 3 ] || {
            printf 'expected: watch SOCKET_PROXY_IMAGE REVISION\n' >&2
            exit 126
        }
        case "$allowed_environment:$#" in
            dev:3 | production:3)
                exec /opt/geoguessme/bin/watch-deploy.sh \
                    "$allowed_environment" "$2" "$3"
                ;;
            *) exit 126 ;;
        esac
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
