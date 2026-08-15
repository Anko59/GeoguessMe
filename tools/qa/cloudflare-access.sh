#!/usr/bin/env bash
set -euo pipefail

# The local QA runner uses this only when a caller did not already provide
# browser Access credentials. The API token creates a short-lived service
# token and an app-local policy; both are removed by the EXIT trap.

qa_access_scratch=''
qa_access_token_id=''
qa_access_policy_id=''

qa_access_cleanup() {
    if declare -F qa_mailbox_cleanup >/dev/null 2>&1; then
        qa_mailbox_cleanup
    fi
    if [[ -n "$qa_access_policy_id" ]]; then
        local policy_response
        policy_response=$(curl --config "$qa_access_scratch/curl.conf" --request DELETE \
            "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps/$qa_access_app_id/policies/$qa_access_policy_id" \
            2>/dev/null) || policy_response=''
        if [[ -z "$policy_response" ]] || ! jq -e '.success == true' >/dev/null 2>&1 <<<"$policy_response"; then
            echo 'Warning: temporary Cloudflare QA policy cleanup failed.' >&2
        fi
    fi
    if [[ -n "$qa_access_token_id" ]]; then
        local token_response
        token_response=$(curl --config "$qa_access_scratch/curl.conf" --request DELETE \
            "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/access/service_tokens/$qa_access_token_id" \
            2>/dev/null) || token_response=''
        if [[ -z "$token_response" ]] || ! jq -e '.success == true' >/dev/null 2>&1 <<<"$token_response"; then
            echo 'Warning: temporary Cloudflare QA service-token cleanup failed.' >&2
        fi
    fi
    if [[ -n "$qa_access_scratch" && -d "$qa_access_scratch" ]]; then
        rm -r -- "$qa_access_scratch"
    fi
}

qa_access_cleanup_on_exit() {
    local exit_code=$?
    qa_access_cleanup
    trap - EXIT
    exit "$exit_code"
}

qa_access_cf_call() {
    local method=$1
    local path=$2
    local payload=${3:-}
    if [[ -n "$payload" ]]; then
        curl --config "$qa_access_scratch/curl.conf" --request "$method" \
            --data "$payload" "https://api.cloudflare.com/client/v4$path"
    else
        curl --config "$qa_access_scratch/curl.conf" --request "$method" \
            "https://api.cloudflare.com/client/v4$path"
    fi
}

qa_access_require_success() {
    if ! jq -e '.success == true' >/dev/null 2>&1 <<<"$1"; then
        echo 'Cloudflare API request for temporary QA access failed.' >&2
        return 1
    fi
}

qa_access_provision() {
    if [[ -z "${QA_ACCESS_CLIENT_ID:-}" && -n "${CF_ACCESS_CLIENT_ID:-}" && -n "${CF_ACCESS_CLIENT_SECRET:-}" ]]; then
        export QA_ACCESS_CLIENT_ID="$CF_ACCESS_CLIENT_ID"
        export QA_ACCESS_CLIENT_SECRET="$CF_ACCESS_CLIENT_SECRET"
    fi
    if [[ -n "${QA_ACCESS_CLIENT_ID:-}" || -n "${QA_ACCESS_CLIENT_SECRET:-}" ]]; then
        if [[ -z "${QA_ACCESS_CLIENT_ID:-}" || -z "${QA_ACCESS_CLIENT_SECRET:-}" ]]; then
            echo 'QA_ACCESS_CLIENT_ID and QA_ACCESS_CLIENT_SECRET must be supplied together.' >&2
            return 2
        fi
        return 0
    fi

    : "${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN is required when QA_ACCESS_* is absent}"
    : "${CLOUDFLARE_ACCOUNT_ID:?CLOUDFLARE_ACCOUNT_ID is required when QA_ACCESS_* is absent}"
    command -v curl >/dev/null || {
        echo 'curl is required for temporary Cloudflare QA access.' >&2
        return 2
    }
    command -v jq >/dev/null || {
        echo 'jq is required for temporary Cloudflare QA access.' >&2
        return 2
    }

    qa_access_scratch=$(mktemp -d "${TMPDIR:-/tmp}/geoguessme-qa-access.XXXXXX")
    chmod 700 "$qa_access_scratch"
    printf '%s\n' \
        'silent' \
        'show-error' \
        "header = \"Authorization: Bearer $CLOUDFLARE_API_TOKEN\"" \
        'header = "Content-Type: application/json"' \
        >"$qa_access_scratch/curl.conf"
    chmod 600 "$qa_access_scratch/curl.conf"
    trap qa_access_cleanup_on_exit EXIT

    local base_host=${QA_BASE_URL#https://}
    base_host=${base_host%%/*}
    base_host=${base_host%%:*}
    local app_id=${QA_ACCESS_APP_ID:-}
    local apps_response
    if [[ -z "$app_id" ]]; then
        apps_response=$(qa_access_cf_call GET "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps")
        qa_access_require_success "$apps_response"
        app_id=$(jq -r --arg host "$base_host" '.result | map(select(.domain == $host)) | .[0].id // empty' <<<"$apps_response")
    fi
    if [[ -z "$app_id" ]]; then
        echo "No Cloudflare Access application matches deployed host $base_host." >&2
        return 1
    fi
    qa_access_app_id=$app_id

    local run_name="GeoGuessMe local QA $(date -u +%Y%m%dT%H%M%SZ)-$$"
    local token_response token_payload
    token_payload=$(jq -cn --arg name "$run_name" '{name: $name, duration: "1h"}')
    token_response=$(qa_access_cf_call POST "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/service_tokens" "$token_payload")
    qa_access_require_success "$token_response"
    qa_access_token_id=$(jq -r '.result.id // empty' <<<"$token_response")
    export QA_ACCESS_CLIENT_ID
    export QA_ACCESS_CLIENT_SECRET
    QA_ACCESS_CLIENT_ID=$(jq -r '.result.client_id // empty' <<<"$token_response")
    QA_ACCESS_CLIENT_SECRET=$(jq -r '.result.client_secret // empty' <<<"$token_response")
    if [[ -z "$qa_access_token_id" || -z "$QA_ACCESS_CLIENT_ID" || -z "$QA_ACCESS_CLIENT_SECRET" ]]; then
        echo 'Cloudflare returned incomplete temporary QA service-token credentials.' >&2
        return 1
    fi

    local policies_response precedence policy_response policy_payload
    policies_response=$(qa_access_cf_call GET "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps/$app_id/policies")
    qa_access_require_success "$policies_response"
    precedence=$(jq -r '[.result[]?.precedence // 0] | max + 1' <<<"$policies_response")
    policy_payload=$(jq -cn \
        --arg name "$run_name" \
        --arg token_id "$qa_access_token_id" \
        --argjson precedence "$precedence" \
        '{name: $name, decision: "non_identity", precedence: $precedence, include: [{service_token: {token_id: $token_id}}]}')
    policy_response=$(qa_access_cf_call POST "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps/$app_id/policies" "$policy_payload")
    qa_access_require_success "$policy_response"
    qa_access_policy_id=$(jq -r '.result.id // empty' <<<"$policy_response")
    if [[ -z "$qa_access_policy_id" ]]; then
        echo 'Cloudflare returned no temporary QA policy identifier.' >&2
        return 1
    fi
}
