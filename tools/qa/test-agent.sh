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
grep -Eq 'source_blind: true' "$root_dir/.agents/qa/policy.yaml"
grep -Eq 'provider_neutral: true' "$root_dir/.agents/qa/tools.yaml"
grep -Eqi 'do not use shell, filesystem, source' "$root_dir/.agents/qa/AGENT.md"
grep -Eq 'no-builtin-tools' "$root_dir/tools/qa/pi-adapter.sh"
grep -Eq -- '--approve-for-me' "$root_dir/tools/qa/codex-adapter.sh"
if grep -Eq -- '--sandbox read-only' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
grep -Eq 'mcp_env_file' "$root_dir/tools/qa/codex-adapter.sh"
grep -Eq 'QA_MAILBOX_ACCESS_CLIENT_ID' "$root_dir/tools/qa/codex-adapter.sh"
grep -Eq 'QA_MAILBOX_ALLOWED_LINK_ORIGINS' "$root_dir/tools/qa/codex-adapter.sh"
grep -Eq 'QA_MAILBOX_ALLOWED_LINK_ORIGINS' "$root_dir/tools/make/tests.mk"
grep -Eq 'QA_AGENT_FOCUS' "$root_dir/tools/qa/codex-adapter.sh"
grep -Eq 'QA_SKIP_BOOTSTRAP' "$root_dir/tools/make/tests.mk"
grep -Eq 'chmod 600' "$root_dir/tools/qa/codex-adapter.sh"
if grep -Eq 'mcp_servers\.qa_browser\.env\.' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
if grep -Eq 'env_vars' "$root_dir/tools/qa/codex-adapter.sh"; then exit 1; fi
grep -Eq 'qa-browser-mcp' "$root_dir/tools/make/tests.mk"
grep -Eq 'mailbox_open_link' "$root_dir/.agents/qa/tools.yaml"
grep -Eq 'browser_transfer_link' "$root_dir/.agents/qa/tools.yaml"
grep -Eq 'browser_open_transferred_link' "$root_dir/.agents/qa/tools.yaml"
qa_recipe=$(sed -n '/^qa-agent: /,/^qa-agent-fast: /p' "$root_dir/tools/make/tests.mk")
grep -q $'\t@QA_BASE_URL=' <<<"$qa_recipe"
if grep -Eq 'QA_ACCOUNT_PASSWORD="\$\(|QA_MAILBOX_ACCESS_CLIENT_SECRET="\$\(' <<<"$qa_recipe"; then
    echo 'QA secret interpolated into a visible Make recipe' >&2
    exit 1
fi
grep -Eq 'qa_account_login' "$root_dir/.agents/qa/AGENT.md"
grep -Eq 'qa_email_account_signup' "$root_dir/.agents/qa/AGENT.md"
grep -Eq 'qa_email_account_signup' "$root_dir/.agents/qa/tools.yaml"
grep -Eq 'runner validates that the operator supplied the dedicated pool password' "$root_dir/.agents/qa/AGENT.md"
grep -Eq 'three distinct dedicated accounts' "$root_dir/.agents/qa/AGENT.md"
grep -Eq 'CLOUDFLARE_API_TOKEN' "$root_dir/tools/qa/cloudflare-access.sh"
if grep -Eq 'CF_ACCESS_CLIENT_ID|CF_ACCESS_CLIENT_SECRET' "$root_dir/tools/qa/cloudflare-access.sh"; then exit 1; fi
grep -Eq 'qa_access_cleanup_on_exit' "$root_dir/tools/qa/cloudflare-access.sh"
grep -Eq 'qa_mailbox_provision' "$root_dir/tools/qa/run-local.sh"
grep -Eq 'QA_ACCOUNT_PASSWORD is required' "$root_dir/tools/qa/run-local.sh"
grep -Eq 'Full and nightly hosted QA require QA_MAILBOX_PROVIDER=cloudflare' "$root_dir/tools/qa/run-local.sh"
grep -Eq 'Email Routing subaddressing' "$root_dir/tools/qa/cloudflare-mailbox.sh"
tagged_address=$(QA_MAILBOX_PROVIDER=cloudflare \
    QA_MAILBOX_ADDRESS=qa-release@example.test \
    QA_MAILBOX_API_URL=https://dev.example.test/_qa-mailbox \
    bash -c 'source "$1/tools/qa/cloudflare-mailbox.sh"; qa_mailbox_provision; printf "%s" "$QA_MAILBOX_ADDRESS"' _ "$root_dir")
[[ "$tagged_address" =~ ^qa-release\+run-[A-Za-z0-9-]+@example\.test$ ]] || {
    echo 'Cloudflare QA mailbox did not receive a fresh tagged address' >&2
    exit 1
}
grep -Eq 'QA_MAILBOX_ACCESS_CLIENT_ID' "$root_dir/tools/qa/browser-mcp.mjs"
if grep -REn --exclude=test-agent.sh 'playwright test|actions/workflows/qa.yml|QA_PASSWORD|QA_USERNAME' "$root_dir/tools/qa" "$root_dir/.agents/qa"; then
    echo 'deterministic QA or committed QA credentials found' >&2
    exit 1
fi
echo 'QA agent contract checks PASSED'
