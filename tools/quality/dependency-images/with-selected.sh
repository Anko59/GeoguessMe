#!/usr/bin/env bash
# Resolve every requested component before exporting any selection or executing
# the consumer. This never prepares images or sources saved shell expressions.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
declare -A selections=() seen=()
while [[ $# -gt 0 && "$1" != -- ]]; do
    component=$1
    shift
    [[ "$component" =~ ^[a-z][a-z0-9-]*$ ]] || {
        echo 'invalid selection component' >&2
        exit 1
    }
    [[ -z "${seen[$component]:-}" ]] || {
        echo 'duplicate selection component' >&2
        exit 1
    }
    seen[$component]=1
    reference=$(bash "$SCRIPT_DIR/selected.sh" "$component") || exit $?
    key=$(printf '%s_IMAGE' "$component" | tr '[:lower:]-' '[:upper:]_')
    selections[$key]=$reference
done
[[ ${#selections[@]} -gt 0 && $# -gt 1 && "$1" == -- ]] || {
    echo 'usage: with-selected.sh COMPONENT ... -- COMMAND [ARGS...]' >&2
    exit 1
}
shift
for key in "${!selections[@]}"; do export "$key=${selections[$key]}"; done
exec "$@"
