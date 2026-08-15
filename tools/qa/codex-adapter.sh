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

# Codex does not inherit the caller's environment into a stdio MCP server.
# Pass the short-lived values through a private, temporary env file instead of
# putting credentials in Codex's process arguments.
mcp_tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/geoguessme-qa-mcp.XXXXXX")
chmod 700 "$mcp_tmp_dir"
mcp_env_file="$mcp_tmp_dir/env"
mcp_wrapper="$mcp_tmp_dir/run-mcp.sh"
agent_output="$mcp_tmp_dir/last-message"
mcp_events="$mcp_tmp_dir/events.jsonl"
mcp_log="$mcp_tmp_dir/mcp-stderr.log"

mcp_cleanup() {
    if [[ -d "$mcp_tmp_dir" ]]; then
        rm -r -- "$mcp_tmp_dir"
    fi
}

mcp_cleanup_on_exit() {
    local exit_code=$?
    mcp_cleanup
    if declare -F qa_access_cleanup >/dev/null 2>&1; then
        qa_access_cleanup
    fi
    trap - EXIT
    exit "$exit_code"
}

trap mcp_cleanup_on_exit EXIT

for env_name in \
    QA_BASE_URL QA_BUILD_SHA QA_REPORT_DIR QA_RUNTIME QA_BUDGET \
    QA_ACCESS_CLIENT_ID QA_ACCESS_CLIENT_SECRET QA_MAILBOX_PROVIDER \
    QA_FAKE_LATITUDE QA_FAKE_LONGITUDE QA_FAKE_LOCATION_ACCURACY \
    QA_ACCOUNT_PASSWORD QA_ACCOUNT_OWNER_USERNAME QA_ACCOUNT_MEMBER_USERNAME \
    QA_ACCOUNT_OUTSIDER_USERNAME; do
    env_value=$(printenv "$env_name" 2>/dev/null || true)
    printf 'export %s=%q\n' "$env_name" "$env_value"
done >"$mcp_env_file"
chmod 600 "$mcp_env_file"

printf '%s\n' \
    '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    "source \"\$1\"" \
    "exec make --no-print-directory -C \"\$2\" qa-browser-mcp 2>\"$mcp_log\"" \
    >"$mcp_wrapper"
chmod 700 "$mcp_wrapper"

mcp_command=$(jq -Rn --arg value "$mcp_wrapper" '$value')
mcp_args=$(jq -cn --arg env_file "$mcp_env_file" --arg root "$root_dir" '[$env_file, $root]')

codex exec \
    --json \
    --output-last-message "$agent_output" \
    --approve-for-me \
    --skip-git-repo-check \
    --ignore-user-config \
    --ignore-rules \
    -C /tmp \
    -c "mcp_servers.qa_browser.command=$mcp_command" \
    -c "mcp_servers.qa_browser.args=$mcp_args" \
    -c 'mcp_servers.qa_browser.cwd="'"$root_dir"'"' \
    - <<EOF >"$mcp_events"
$prompt
EOF

if [[ ! -s "$QA_REPORT_DIR/qa-report.json" ]]; then
    if [[ "${QA_DEBUG:-}" == 1 && -s "$agent_output" ]]; then
        cp -- "$agent_output" "$QA_REPORT_DIR/qa-agent-last-message.txt"
    fi
    if [[ "${QA_DEBUG:-}" == 1 && -s "$mcp_log" ]]; then
        cp -- "$mcp_log" "$QA_REPORT_DIR/qa-mcp-stderr.log"
    fi
    echo 'QA agent did not produce qa-report.json; treating the run as a harness failure.' >&2
    exit 1
fi

if jq -e '.summary == "The runtime ended before qa_finish was called."' "$QA_REPORT_DIR/qa-report.json" >/dev/null; then
    if [[ "${QA_DEBUG:-}" == 1 && -s "$mcp_events" ]]; then
        cp -- "$mcp_events" "$QA_REPORT_DIR/qa-agent-events.jsonl"
    fi
    echo 'QA agent ended without calling qa_finish; treating the run as a harness failure.' >&2
    exit 1
fi
