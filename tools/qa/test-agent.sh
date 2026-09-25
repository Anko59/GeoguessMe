#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "$0")/../.." && pwd)
test -f "$root_dir/.agents/qa/AGENT.md"
test -f "$root_dir/.agents/qa/policy.yaml"
test -f "$root_dir/.agents/qa/tools.yaml"
test -f "$root_dir/.agents/qa/mcp.json"
test -x "$root_dir/tools/qa/cloudflare-access.sh"
test -x "$root_dir/tools/qa/cloudflare-mailbox.sh"
test -f "$root_dir/tools/qa/browser-capabilities.mjs"
test -f "$root_dir/tools/qa/coverage.mjs"
test -f "$root_dir/tools/qa/mcp-schemas.mjs"
test -f "$root_dir/tools/qa/mailbox.mjs"
test -f "$root_dir/tools/qa/account-pool.mjs"
test -f "$root_dir/tools/qa/test-account-pool.mjs"
test -f "$root_dir/tools/qa/email-account.mjs"
test -f "$root_dir/tools/qa/test-email-account.mjs"
test -f "$root_dir/tools/qa/browser/tool-definitions.mjs"
test -f "$root_dir/tools/qa/test-coverage.mjs"
test -f "$root_dir/tools/qa/safe-output.mjs"
test ! -e "$root_dir/.github/workflows/qa.yml"
rg -q 'source_blind: true' "$root_dir/.agents/qa/policy.yaml"
rg -q 'provider_neutral: true' "$root_dir/.agents/qa/tools.yaml"
rg -qi 'do not use shell, filesystem, source' "$root_dir/.agents/qa/AGENT.md"
rg -q 'no-builtin-tools' "$root_dir/tools/qa/pi-adapter.sh"
rg -q -- '--approve-for-me' "$root_dir/tools/qa/codex-adapter.sh"
if rg -q -- '--sandbox read-only' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
rg -q 'mcp_env_file' "$root_dir/tools/qa/codex-adapter.sh"
rg -q 'QA_MAILBOX_ACCESS_CLIENT_ID' "$root_dir/tools/qa/codex-adapter.sh"
rg -q 'QA_MAILBOX_ALLOWED_LINK_ORIGINS' "$root_dir/tools/qa/codex-adapter.sh"
rg -q 'QA_MAILBOX_ALLOWED_LINK_ORIGINS' "$root_dir/tools/make/tests.mk"
rg -q 'QA_AGENT_FOCUS' "$root_dir/tools/qa/codex-adapter.sh"
rg -q 'QA_SKIP_BOOTSTRAP' "$root_dir/tools/make/tests.mk"
rg -q 'chmod 600' "$root_dir/tools/qa/codex-adapter.sh"
if rg -q 'mcp_servers\.qa_browser\.env\.' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
if rg -q 'env_vars' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
rg -q 'qa-browser-mcp' "$root_dir/tools/make/tests.mk"
rg -q 'mailbox_open_link' "$root_dir/.agents/qa/tools.yaml"
rg -q 'browser_transfer_link' "$root_dir/.agents/qa/tools.yaml"
rg -q 'browser_open_transferred_link' "$root_dir/.agents/qa/tools.yaml"
qa_recipe=$(sed -n '/^qa-agent: /,/^qa-agent-fast: /p' "$root_dir/tools/make/tests.mk")
grep -q $'\t@QA_BASE_URL=' <<<"$qa_recipe"
rg -q 'qa_account_login' "$root_dir/.agents/qa/AGENT.md"
rg -q 'qa_email_account_signup' "$root_dir/.agents/qa/AGENT.md"
rg -q 'qa_email_account_signup' "$root_dir/.agents/qa/tools.yaml"
rg -q 'runner validates that the operator supplied the dedicated pool password' "$root_dir/.agents/qa/AGENT.md"
rg -q 'three distinct dedicated accounts' "$root_dir/.agents/qa/AGENT.md"
rg -q 'CLOUDFLARE_API_TOKEN' "$root_dir/tools/qa/cloudflare-access.sh"
if rg -q 'CF_ACCESS_CLIENT_ID|CF_ACCESS_CLIENT_SECRET' "$root_dir/tools/qa/cloudflare-access.sh"; then exit 1; fi
rg -q 'qa_access_cleanup_on_exit' "$root_dir/tools/qa/cloudflare-access.sh"
rg -q 'qa_mailbox_provision' "$root_dir/tools/qa/run-local.sh"
rg -q 'QA_ACCOUNT_PASSWORD is required' "$root_dir/tools/qa/run-local.sh"
rg -q 'Full and nightly hosted QA require QA_MAILBOX_PROVIDER=cloudflare' "$root_dir/tools/qa/run-local.sh"
rg -q 'temporary literal Email Routing rule' "$root_dir/tools/qa/cloudflare-mailbox.sh"
rg -q 'QA_MAILBOX_ACCESS_CLIENT_ID' "$root_dir/tools/qa/browser-mcp.mjs"
if rg -n 'playwright test|actions/workflows/qa.yml|QA_PASSWORD|QA_USERNAME' "$root_dir/tools/qa" "$root_dir/.agents/qa" --glob '!test-agent.sh'; then
    echo 'deterministic QA or committed QA credentials found' >&2
    exit 1
fi
echo 'QA agent contract checks PASSED'
