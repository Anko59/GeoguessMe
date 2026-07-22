#!/bin/sh
set -eu
set -f

allowed_environment=${1:-}
case "$allowed_environment" in dev | production) ;; *) exit 126 ;; esac

# SSH_ORIGINAL_COMMAND is untrusted. Parse a deliberately tiny protocol and
# reject shell metacharacters through strict per-field validation in deploy.sh.
# shellcheck disable=SC2086
set -- ${SSH_ORIGINAL_COMMAND:-}
[ "$#" -eq 4 ] || {
    printf 'expected: deploy BACKEND_IMAGE WEB_IMAGE REVISION\n' >&2
    exit 126
}
[ "$1" = deploy ] || exit 126
exec /opt/geoguessme/bin/deploy.sh "$allowed_environment" "$2" "$3" "$4"
