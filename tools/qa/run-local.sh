#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "$0")/../.." && pwd)
runtime=$(printenv QA_RUNTIME 2>/dev/null || printf '%s' codex)
budget=$(printenv QA_BUDGET 2>/dev/null || printf '%s' full)
base_url=$(printenv QA_BASE_URL 2>/dev/null || true)
report_dir=$(printenv QA_REPORT_DIR 2>/dev/null || printf '%s' "$root_dir/qa-artifacts")

# The operator keyring is shared across worktrees; a repository-local .envrc
# is not. Explicit environment values still take precedence.
if [[ -z "${QA_ACCOUNT_PASSWORD:-}" ]] && command -v secret-tool >/dev/null 2>&1; then
    export QA_ACCOUNT_PASSWORD
    QA_ACCOUNT_PASSWORD=$(secret-tool lookup service codex-api provider qa project geoguessme name QA_ACCOUNT_PASSWORD 2>/dev/null || true)
fi

if [[ -z "$base_url" ]]; then
    echo "QA_BASE_URL is required and must be the deployed dev URL." >&2
    exit 2
fi
if [[ -z "${QA_ACCOUNT_PASSWORD:-}" ]]; then
    echo "QA_ACCOUNT_PASSWORD is required for the dedicated dev QA account pool." >&2
    echo "Load it from the operator keyring before starting an agent; see docs/qa-agent.md." >&2
    exit 2
fi
if [[ ! "$base_url" =~ ^https:// ]] && [[ "$(printenv QA_ALLOW_LOCAL 2>/dev/null || true)" != 1 ]]; then
    echo "QA_BASE_URL must use https for a deployed environment." >&2
    exit 2
fi
if [[ "$base_url" =~ ^https://(localhost|127\.0\.0\.1)(:|/) ]] && [[ "$(printenv QA_ALLOW_LOCAL 2>/dev/null || true)" != 1 ]]; then
    echo "QA_BASE_URL points at localhost; this command is for deployed dev." >&2
    exit 2
fi
case "$budget" in
    fast | full | nightly) ;;
    *)
        echo "QA_BUDGET must be fast, full, or nightly." >&2
        exit 2
        ;;
esac
if [[ "$budget" != fast && "$base_url" =~ ^https://dev\.geoguessme\.com/?$ ]]; then
    export QA_MAILBOX_PROVIDER=${QA_MAILBOX_PROVIDER:-cloudflare}
    export QA_MAILBOX_API_URL=${QA_MAILBOX_API_URL:-https://dev.geoguessme.com/_qa-mailbox}
    export QA_MAILBOX_ADDRESS=${QA_MAILBOX_ADDRESS:-qa-release-20260815-77679@geoguessme.com}
    export QA_MAILBOX_ALLOWED_LINK_ORIGINS=${QA_MAILBOX_ALLOWED_LINK_ORIGINS:-https://auth.geoguessme.com}
fi
if [[ "$budget" != fast && "${QA_MAILBOX_PROVIDER:-mailtm}" != cloudflare ]]; then
    echo "Full and nightly hosted QA require QA_MAILBOX_PROVIDER=cloudflare and the controlled relay configuration." >&2
    echo "See docs/qa-agent.md; the public mailbox fallback is not accepted as release evidence." >&2
    exit 2
fi
case "$runtime" in
    codex | pi) ;;
    *)
        echo "QA_RUNTIME must be codex or pi." >&2
        exit 2
        ;;
esac

mkdir -p "$report_dir"
chmod 700 "$report_dir"
export QA_BASE_URL QA_BUDGET QA_REPORT_DIR
build_sha=$(printenv QA_BUILD_SHA 2>/dev/null || true)
if [[ ! "$build_sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo 'QA_BUILD_SHA must be the full SHA of the revision actually deployed at QA_BASE_URL.' >&2
    echo 'The checkout revision is not deployment evidence; see docs/qa-agent.md.' >&2
    exit 2
fi
export QA_BUILD_SHA="$build_sha"
export QA_RUNTIME="$runtime"

if [[ -z "${CLOUDFLARE_API_TOKEN:-}" ]] && command -v secret-tool >/dev/null 2>&1; then
    export CLOUDFLARE_API_TOKEN
    CLOUDFLARE_API_TOKEN=$(secret-tool lookup service codex-api provider cloudflare project geoguessme name CLOUDFLARE_API_TOKEN 2>/dev/null || true)
fi
export CLOUDFLARE_ACCOUNT_ID=${CLOUDFLARE_ACCOUNT_ID:-10f05054710460b4662ba876f08d5689}

# Use caller-supplied browser credentials when available. Otherwise derive a
# short-lived dev-scoped Access credential from the existing Cloudflare API
# token and clean it up when this local run exits.
source "$root_dir/tools/qa/cloudflare-access.sh"
source "$root_dir/tools/qa/cloudflare-mailbox.sh"
qa_access_provision
qa_mailbox_provision

case "$runtime" in
    codex) "$root_dir/tools/qa/codex-adapter.sh" "$root_dir" ;;
    pi) "$root_dir/tools/qa/pi-adapter.sh" "$root_dir" ;;
esac
