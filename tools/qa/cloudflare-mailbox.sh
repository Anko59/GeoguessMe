#!/usr/bin/env bash
set -euo pipefail

# A hosted QA mailbox uses a temporary literal Email Routing rule. The seed
# address identifies the dedicated relay domain; each run receives a unique
# address so an already-verified account can never make a later run look like
# an application verification failure.

qa_mailbox_scratch=''
qa_mailbox_rule_id=''
qa_mailbox_zone_id=''

qa_mailbox_cf_call() {
    local method=$1
    local path=$2
    local payload=${3:-}
    if [[ -n "$payload" ]]; then
        curl --config "$qa_mailbox_scratch/curl.conf" --request "$method" \
            --data "$payload" "https://api.cloudflare.com/client/v4$path"
    else
        curl --config "$qa_mailbox_scratch/curl.conf" --request "$method" \
            "https://api.cloudflare.com/client/v4$path"
    fi
}

qa_mailbox_require_success() {
    if ! jq -e '.success == true' >/dev/null 2>&1 <<<"$1"; then
        echo 'Cloudflare API request for the temporary QA mailbox route failed.' >&2
        return 1
    fi
}

qa_mailbox_provision() {
    [[ "${QA_MAILBOX_PROVIDER:-}" == cloudflare ]] || return 0
    : "${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN is required for the Cloudflare QA mailbox relay}"
    : "${QA_MAILBOX_ADDRESS:?QA_MAILBOX_ADDRESS must be the dedicated relay seed address}"
    : "${QA_MAILBOX_ZONE_ID:?QA_MAILBOX_ZONE_ID is required for the Cloudflare QA mailbox relay}"
    : "${QA_MAILBOX_ROUTING_RULE_ID:?QA_MAILBOX_ROUTING_RULE_ID is required for the Cloudflare QA mailbox relay}"
    command -v curl >/dev/null || {
        echo 'curl is required for the Cloudflare QA mailbox relay.' >&2
        return 2
    }
    command -v jq >/dev/null || {
        echo 'jq is required for the Cloudflare QA mailbox relay.' >&2
        return 2
    }

    local seed_local seed_domain
    if [[ "$QA_MAILBOX_ADDRESS" =~ ^([A-Za-z0-9][A-Za-z0-9._+-]*)@([A-Za-z0-9.-]+)$ ]]; then
        seed_local=${BASH_REMATCH[1]}
        seed_domain=${BASH_REMATCH[2]}
    else
        echo 'QA_MAILBOX_ADDRESS must be a valid email address.' >&2
        return 2
    fi

    qa_mailbox_zone_id="$QA_MAILBOX_ZONE_ID"

    qa_mailbox_scratch=$(mktemp -d "${TMPDIR:-/tmp}/geoguessme-qa-mailbox.XXXXXX")
    chmod 700 "$qa_mailbox_scratch"
    printf '%s\n' \
        'silent' \
        'show-error' \
        "header = \"Authorization: Bearer $CLOUDFLARE_API_TOKEN\"" \
        'header = "Content-Type: application/json"' \
        >"$qa_mailbox_scratch/curl.conf"
    chmod 600 "$qa_mailbox_scratch/curl.conf"

    local rule_response worker_name rule_name suffix unique_local unique_address payload create_response
    rule_response=$(qa_mailbox_cf_call GET "/zones/$QA_MAILBOX_ZONE_ID/email/routing/rules/$QA_MAILBOX_ROUTING_RULE_ID")
    qa_mailbox_require_success "$rule_response"
    worker_name=$(jq -r '.result.actions | map(select(.type == "worker")) | .[0].value[0] // empty' <<<"$rule_response")
    rule_name=$(jq -r '.result.name // "geoguessme-qa-mailbox"' <<<"$rule_response")
    if [[ -z "$worker_name" ]]; then
        echo 'The configured QA mailbox routing rule does not target a Worker.' >&2
        return 1
    fi

    suffix="$(date -u +%Y%m%d%H%M%S)-${BASHPID}-${RANDOM}"
    unique_local="${seed_local}-run-${suffix}"
    unique_address="${unique_local}@${seed_domain}"
    payload=$(jq -cn \
        --arg name "${rule_name}-run-${suffix}" \
        --arg address "$unique_address" \
        --arg worker "$worker_name" \
        '{name: $name, enabled: true, matchers: [{type: "literal", field: "to", value: $address}], actions: [{type: "worker", value: [$worker]}]}')
    create_response=$(qa_mailbox_cf_call POST "/zones/$QA_MAILBOX_ZONE_ID/email/routing/rules" "$payload")
    qa_mailbox_require_success "$create_response"
    qa_mailbox_rule_id=$(jq -r '.result.id // empty' <<<"$create_response")
    if [[ -z "$qa_mailbox_rule_id" ]]; then
        echo 'Cloudflare returned no temporary QA mailbox routing-rule ID.' >&2
        return 1
    fi
    export QA_MAILBOX_ADDRESS="$unique_address"
}

qa_mailbox_cleanup() {
    if [[ -n "$qa_mailbox_rule_id" ]]; then
        local response
        response=$(qa_mailbox_cf_call DELETE "/zones/$qa_mailbox_zone_id/email/routing/rules/$qa_mailbox_rule_id" 2>/dev/null) || response=''
        if [[ -z "$response" ]] || ! jq -e '.success == true' >/dev/null 2>&1 <<<"$response"; then
            echo 'Warning: temporary Cloudflare QA mailbox routing-rule cleanup failed.' >&2
        fi
    fi
    if [[ -n "$qa_mailbox_scratch" && -d "$qa_mailbox_scratch" ]]; then
        rm -r -- "$qa_mailbox_scratch"
    fi
    qa_mailbox_rule_id=''
    qa_mailbox_zone_id=''
    qa_mailbox_scratch=''
}
