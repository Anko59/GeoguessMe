#!/usr/bin/env bash
set -euo pipefail

# Share the installed hosted runtime helper, including old-release support.
# Only the two public templates change; secrets remain runtime env values.
source_root="$(cd "$(dirname "$0")/../.." && pwd -P)"
# shellcheck source=deployment/scripts/hosted/common.sh
source "$source_root/deployment/scripts/hosted/common.sh"
prepare_public_configs "${1:-$source_root}"
