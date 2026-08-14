#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -lt 1 ]]; then
    echo "repository root is required" >&2
    exit 2
fi
root_dir=$1
prompt_file="$root_dir/.agents/qa/AGENT.md"
prompt=$(<"$prompt_file")
prompt+=$'\n\nRun context:\n- Target deployed URL: '"$QA_BASE_URL"$'\n- Deployed revision evidence: '"$QA_BUILD_SHA"$'\n- Budget: '"$QA_BUDGET"$'\n\nUse only the qa-browser MCP tools. Start with session_create and finish with qa_finish. Do not use any built-in source, shell, filesystem, HTTP, or web tools even if they are offered by the runtime.'

codex exec \
    --ephemeral \
    --json \
    --sandbox read-only \
    --skip-git-repo-check \
    --ignore-user-config \
    --ignore-rules \
    -C /tmp \
    -c 'mcp_servers.qa_browser.command="make"' \
    -c 'mcp_servers.qa_browser.args=["--no-print-directory","-C","'"$root_dir"'","qa-browser-mcp"]' \
    -c 'mcp_servers.qa_browser.cwd="'"$root_dir"'"' \
    -c 'mcp_servers.qa_browser.env_vars=["QA_BASE_URL","QA_BUILD_SHA","QA_REPORT_DIR","QA_RUNTIME","QA_BUDGET","QA_ACCESS_CLIENT_ID","QA_ACCESS_CLIENT_SECRET","QA_MAILBOX_PROVIDER","QA_FAKE_LATITUDE","QA_FAKE_LONGITUDE","QA_FAKE_LOCATION_ACCURACY","QA_ACCOUNT_PASSWORD","QA_ACCOUNT_OWNER_USERNAME","QA_ACCOUNT_MEMBER_USERNAME","QA_ACCOUNT_OUTSIDER_USERNAME"]' \
    -c 'mcp_servers.qa_browser.default_tools_approval_mode="approve"' \
    - <<EOF >/dev/null
$prompt
EOF
