#!/usr/bin/env bash
set -euo pipefail

readonly DEFAULT_CLOUDFLARE_ACCOUNT_ID=10f05054710460b4662ba876f08d5689
readonly KEYRING_SERVICE=codex-api
readonly KEYRING_PROVIDER=cloudflare
readonly KEYRING_PROJECT=geoguessme
readonly KEYRING_NAME=CLOUDFLARE_API_TOKEN

credential_source=''
scratch_dir=''
access_app_id=''
access_token_id=''
access_policy_id=''

# The API token is read by this process only. Do not pass it to helper commands.
export -n CLOUDFLARE_API_TOKEN 2>/dev/null || true

resolve_api_token() {
    if [[ -n "${CLOUDFLARE_API_TOKEN:-}" ]]; then
        credential_source='environment'
        return 0
    fi

    if command -v secret-tool >/dev/null 2>&1; then
        CLOUDFLARE_API_TOKEN=$(secret-tool lookup \
            service "$KEYRING_SERVICE" \
            provider "$KEYRING_PROVIDER" \
            project "$KEYRING_PROJECT" \
            name "$KEYRING_NAME" 2>/dev/null || true)
        if [[ -n "$CLOUDFLARE_API_TOKEN" ]]; then
            credential_source='GNOME Secret Service keyring'
            return 0
        fi
    fi

    return 1
}

credentials_preflight() {
    local missing=0

    if resolve_api_token; then
        printf 'Cloudflare API token: available via %s\n' "$credential_source"
    else
        printf 'Cloudflare API token: unavailable (checked environment and Secret Service item %s/%s/%s/%s)\n' \
            "$KEYRING_SERVICE" "$KEYRING_PROVIDER" "$KEYRING_PROJECT" "$KEYRING_NAME"
        missing=1
    fi

    if command -v ssh-add >/dev/null 2>&1 && ssh-add -l >/dev/null 2>&1; then
        printf 'Operator SSH identity: available from ssh-agent\n'
    else
        printf 'Operator SSH identity: unavailable (load the operator key into ssh-agent)\n'
        missing=1
    fi

    if command -v gh >/dev/null 2>&1 && gh auth status --hostname github.com >/dev/null 2>&1; then
        printf 'GitHub CLI authentication: available\n'
    else
        printf 'GitHub CLI authentication: unavailable (optional for operator SSH)\n'
    fi

    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        printf 'Docker Compose: available\n'
    else
        printf 'Docker Compose: unavailable\n'
        missing=1
    fi

    for command_name in curl jq ssh make; do
        if command -v "$command_name" >/dev/null 2>&1; then
            printf '%s: available\n' "$command_name"
        else
            printf '%s: unavailable\n' "$command_name"
            missing=1
        fi
    done

    printf 'GitHub environment secret values: not readable by local gh; workflows consume them directly\n'
    return "$missing"
}

api_request() {
    local method=$1
    local path=$2
    local payload=${3:-}
    local url="https://api.cloudflare.com/client/v4$path"

    if [[ -n "$payload" ]]; then
        curl --config "$scratch_dir/api-curl.conf" \
            --request "$method" --data "$payload" "$url"
    else
        curl --config "$scratch_dir/api-curl.conf" \
            --request "$method" "$url"
    fi
}

require_api_success() {
    if ! jq -e '.success == true' >/dev/null 2>&1 <<<"$1"; then
        echo 'Cloudflare Access API operation failed.' >&2
        return 1
    fi
}

delete_access_resource() {
    local path=$1
    local response

    if ! response=$(api_request DELETE "$path" 2>/dev/null) ||
        ! jq -e '.success == true' >/dev/null 2>&1 <<<"$response"; then
        echo 'Warning: temporary Cloudflare Access cleanup failed; the token expires within one hour.' >&2
        return 1
    fi
}

cleanup_access_session() {
    local exit_code=$?
    trap - EXIT INT TERM

    if [[ -n "$access_policy_id" ]]; then
        delete_access_resource "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/apps/$access_app_id/policies/$access_policy_id" || true
    fi
    if [[ -n "$access_token_id" ]]; then
        delete_access_resource "/accounts/$CLOUDFLARE_ACCOUNT_ID/access/service_tokens/$access_token_id" || true
    fi

    unset TUNNEL_SERVICE_TOKEN_ID TUNNEL_SERVICE_TOKEN_SECRET
    if [[ -n "$scratch_dir" && -d "$scratch_dir" ]]; then
        rm -rf -- "$scratch_dir"
    fi
    exit "$exit_code"
}

wait_for_access_policy() {
    local hostname=$1
    local status='000'
    local deadline=$((SECONDS + 30))

    while ((SECONDS < deadline)); do
        if status=$(curl --config "$scratch_dir/access-curl.conf" \
            --silent --show-error --max-time 5 --output /dev/null \
            --write-out '%{http_code}' "https://$hostname/" 2>/dev/null); then
            if [[ "$status" == 200 ]]; then
                printf 'Cloudflare Access is ready for %s\n' "$hostname"
                return 0
            fi
        else
            status='000'
        fi
        sleep 1
    done

    printf 'Cloudflare Access policy did not become ready for %s within 30 seconds (last HTTP status %s).\n' \
        "$hostname" "$status" >&2
    return 1
}

operator_ssh() {
    local environment=$1
    local hostname
    local account_id=${CLOUDFLARE_ACCOUNT_ID:-$DEFAULT_CLOUDFLARE_ACCOUNT_ID}
    local applications_response
    local application_count
    local policies_response
    local policy_precedence
    local token_payload
    local token_response
    local policy_payload
    local policy_response
    local timestamp
    local proxy_command
    local ssh_status

    case "$environment" in
        dev) hostname='deploy.geoguessme.com' ;;
        production) hostname='deploy-prod.geoguessme.com' ;;
        *)
            echo 'HOST must be dev or production.' >&2
            return 2
            ;;
    esac

    if ! resolve_api_token; then
        echo 'Cloudflare API token is unavailable; run make credentials-preflight and follow the credential-access runbook.' >&2
        return 2
    fi
    for command_name in curl jq ssh make; do
        command -v "$command_name" >/dev/null 2>&1 || {
            printf '%s is required for operator SSH.\n' "$command_name" >&2
            return 2
        }
    done
    if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
        echo 'Docker Compose is required for the pinned Cloudflare Access SSH proxy.' >&2
        return 2
    fi
    if ! command -v ssh-add >/dev/null 2>&1 || ! ssh-add -l >/dev/null 2>&1; then
        echo 'Load the operator SSH identity into ssh-agent before connecting.' >&2
        return 2
    fi

    export CLOUDFLARE_ACCOUNT_ID=$account_id
    scratch_dir=$(mktemp -d /tmp/geoguessme-ops-access.XXXXXX)
    chmod 700 "$scratch_dir"
    printf '%s\n' \
        'silent' \
        'show-error' \
        "header = \"Authorization: Bearer $CLOUDFLARE_API_TOKEN\"" \
        'header = "Content-Type: application/json"' \
        >"$scratch_dir/api-curl.conf"
    chmod 600 "$scratch_dir/api-curl.conf"
    trap cleanup_access_session EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    unset CLOUDFLARE_API_TOKEN

    if ! applications_response=$(api_request GET "/accounts/$account_id/access/apps"); then
        echo 'Could not list the Cloudflare Access applications.' >&2
        return 1
    fi
    require_api_success "$applications_response"
    application_count=$(jq -r --arg hostname "$hostname" \
        '[.result[]? | select(.domain == $hostname)] | length' <<<"$applications_response")
    if [[ "$application_count" != 1 ]]; then
        printf 'Expected one Cloudflare Access application for %s; found %s.\n' \
            "$hostname" "$application_count" >&2
        return 1
    fi
    access_app_id=$(jq -r --arg hostname "$hostname" \
        '.result[] | select(.domain == $hostname) | .id' <<<"$applications_response")

    timestamp=$(date -u +%Y%m%dT%H%M%SZ)
    token_payload=$(jq -cn --arg name "GeoGuessMe local operator $environment $timestamp" \
        '{name: $name, duration: "1h"}')
    if ! token_response=$(api_request POST "/accounts/$account_id/access/service_tokens" "$token_payload"); then
        echo 'Could not create a temporary Cloudflare Access service token.' >&2
        return 1
    fi
    require_api_success "$token_response"
    access_token_id=$(jq -r '.result.id // empty' <<<"$token_response")
    export TUNNEL_SERVICE_TOKEN_ID
    export TUNNEL_SERVICE_TOKEN_SECRET
    TUNNEL_SERVICE_TOKEN_ID=$(jq -r '.result.client_id // empty' <<<"$token_response")
    TUNNEL_SERVICE_TOKEN_SECRET=$(jq -r '.result.client_secret // empty' <<<"$token_response")
    if [[ -z "$access_token_id" || -z "$TUNNEL_SERVICE_TOKEN_ID" || -z "$TUNNEL_SERVICE_TOKEN_SECRET" ]]; then
        echo 'Cloudflare returned incomplete temporary service-token credentials.' >&2
        return 1
    fi

    if ! policies_response=$(api_request GET "/accounts/$account_id/access/apps/$access_app_id/policies"); then
        echo 'Could not list the selected application policies.' >&2
        return 1
    fi
    require_api_success "$policies_response"
    policy_precedence=$(jq -r '[.result[]?.precedence // 0] | (max // 0) + 1' <<<"$policies_response")
    policy_payload=$(jq -cn \
        --arg name "GeoGuessMe local operator $environment $timestamp" \
        --arg token_id "$access_token_id" \
        --argjson precedence "$policy_precedence" \
        '{name: $name, decision: "non_identity", precedence: $precedence, include: [{service_token: {token_id: $token_id}}]}')
    if ! policy_response=$(api_request POST "/accounts/$account_id/access/apps/$access_app_id/policies" "$policy_payload"); then
        echo 'Could not create the temporary application-scoped Access policy.' >&2
        return 1
    fi
    require_api_success "$policy_response"
    access_policy_id=$(jq -r '.result.id // empty' <<<"$policy_response")
    if [[ -z "$access_policy_id" ]]; then
        echo 'Cloudflare returned no temporary Access policy identifier.' >&2
        return 1
    fi

    printf '%s\n' \
        "header = \"CF-Access-Client-Id: $TUNNEL_SERVICE_TOKEN_ID\"" \
        "header = \"CF-Access-Client-Secret: $TUNNEL_SERVICE_TOKEN_SECRET\"" \
        >"$scratch_dir/access-curl.conf"
    chmod 600 "$scratch_dir/access-curl.conf"
    wait_for_access_policy "$hostname"

    proxy_command="make -s cloudflared-access-ssh HOST=%h 2>'$scratch_dir/cloudflared.stderr'"
    if [[ -n "${OPS_SSH_COMMAND:-}" ]]; then
        if ssh -o BatchMode=yes -o ConnectTimeout=20 -o "ProxyCommand=$proxy_command" \
            "ops@$hostname" "$OPS_SSH_COMMAND"; then
            ssh_status=0
        else
            ssh_status=$?
        fi
    else
        if ssh -o BatchMode=yes -o ConnectTimeout=20 -o "ProxyCommand=$proxy_command" \
            "ops@$hostname"; then
            ssh_status=0
        else
            ssh_status=$?
        fi
    fi

    if ((ssh_status != 0)); then
        if grep -Fq '/cdn-cgi/access/cli' "$scratch_dir/cloudflared.stderr" 2>/dev/null; then
            echo 'Cloudflare requested interactive login instead of accepting the temporary service token.' >&2
        else
            echo 'Operator SSH through Cloudflare Access failed; temporary credentials will be removed.' >&2
        fi
    fi
    return "$ssh_status"
}

case "${1:-}" in
    preflight)
        credentials_preflight
        ;;
    ssh)
        [[ $# -ge 2 ]] || {
            echo 'Usage: credentials.sh ssh dev|production' >&2
            exit 2
        }
        shift
        operator_ssh "$@"
        ;;
    *)
        echo 'Usage: credentials.sh preflight | ssh dev|production' >&2
        exit 2
        ;;
esac
