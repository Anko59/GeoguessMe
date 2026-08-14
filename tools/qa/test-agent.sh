#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "$0")/../.." && pwd)
test -f "$root_dir/.agents/qa/AGENT.md"
test -f "$root_dir/.agents/qa/policy.yaml"
test -f "$root_dir/.agents/qa/tools.yaml"
test -f "$root_dir/.agents/qa/mcp.json"
test -x "$root_dir/tools/qa/cloudflare-access.sh"
test -f "$root_dir/tools/qa/browser-capabilities.mjs"
test -f "$root_dir/tools/qa/mailbox.mjs"
test -f "$root_dir/tools/qa/safe-output.mjs"
test ! -e "$root_dir/.github/workflows/qa.yml"
rg -q 'source_blind: true' "$root_dir/.agents/qa/policy.yaml"
rg -q 'provider_neutral: true' "$root_dir/.agents/qa/tools.yaml"
rg -qi 'do not use shell, filesystem, source' "$root_dir/.agents/qa/AGENT.md"
rg -q 'no-builtin-tools' "$root_dir/tools/qa/pi-adapter.sh"
rg -q -- '--sandbox read-only' "$root_dir/tools/qa/codex-adapter.sh"
rg -q 'qa-browser-mcp' "$root_dir/tools/make/tests.mk"
rg -q 'mailbox_open_link' "$root_dir/.agents/qa/tools.yaml"
rg -q 'browser_transfer_link' "$root_dir/.agents/qa/tools.yaml"
rg -q 'browser_open_transferred_link' "$root_dir/.agents/qa/tools.yaml"
rg -q 'three distinct mailboxes' "$root_dir/.agents/qa/AGENT.md"
rg -q 'CLOUDFLARE_API_TOKEN' "$root_dir/tools/qa/cloudflare-access.sh"
rg -q 'qa_access_cleanup_on_exit' "$root_dir/tools/qa/cloudflare-access.sh"
if rg -n 'playwright test|actions/workflows/qa.yml|QA_PASSWORD|QA_USERNAME' "$root_dir/tools/qa" "$root_dir/.agents/qa" --glob '!test-agent.sh'; then
    echo 'deterministic QA or committed QA credentials found' >&2
    exit 1
fi
echo 'QA agent contract checks PASSED'
