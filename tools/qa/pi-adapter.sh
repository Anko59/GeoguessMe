#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -lt 1 ]]; then
    echo "repository root is required" >&2
    exit 2
fi
root_dir=$1
prompt_file="$root_dir/.agents/qa/AGENT.md"
prompt=$(<"$prompt_file")
prompt+=$'\n\nRun context:\n- Target deployed URL: '"$QA_BASE_URL"$'\n- Deployed revision evidence: '"$QA_BUILD_SHA"$'\n- Budget: '"$QA_BUDGET"$'\n\nUse only the qa-browser MCP tools. Start with session_create and finish with qa_finish. Do not use built-in source, shell, filesystem, HTTP, or web tools.'
pi_model=$(printenv QA_PI_MODEL 2>/dev/null || true)
pi_provider=$(printenv QA_PI_PROVIDER 2>/dev/null || printf '%s' openai-codex)
set -- \
    --print --mode json --no-session --no-context-files --no-skills \
    --no-prompt-templates --no-themes --no-builtin-tools \
    --mcp-config "$root_dir/.agents/qa/mcp.json" \
    --provider "$pi_provider"
if [[ -n "$pi_model" ]]; then
    set -- "$@" --model "$pi_model"
fi
pi "$@" "$prompt" >/dev/null
