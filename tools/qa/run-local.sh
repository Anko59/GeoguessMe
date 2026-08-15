#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "$0")/../.." && pwd)
runtime=$(printenv QA_RUNTIME 2>/dev/null || printf '%s' codex)
budget=$(printenv QA_BUDGET 2>/dev/null || printf '%s' full)
base_url=$(printenv QA_BASE_URL 2>/dev/null || true)
report_dir=$(printenv QA_REPORT_DIR 2>/dev/null || printf '%s' "$root_dir/qa-artifacts")

if [[ -z "$base_url" ]]; then
    echo "QA_BASE_URL is required and must be the deployed dev URL." >&2
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
if [[ -z "$build_sha" ]]; then
    build_sha=$(git -C "$root_dir" rev-parse origin/dev 2>/dev/null || git -C "$root_dir" rev-parse HEAD)
fi
export QA_BUILD_SHA="$build_sha"
export QA_RUNTIME="$runtime"

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
