#!/usr/bin/env bash
# Pure content-reference lookup: no daemon, registry or application revision.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
[[ $# == 2 ]] || fail 'usage: image-ref.sh local|remote COMPONENT'
load_component "$2"
case "$1" in
    local) printf '%s\n' "$LOCAL_REF" ;;
    remote) printf '%s\n' "$REMOTE_REF" ;;
    *) fail 'mode must be local or remote' ;;
esac
