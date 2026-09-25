#!/usr/bin/env bash
set -euo pipefail

# Cloudflare Email Routing subaddressing matches seed+tag@domain against the
# existing literal seed@domain rule. The Worker retains the full recipient in
# its KV key, so every run gets an isolated inbox without changing routing
# rules or requiring Email Routing API permissions.

qa_mailbox_provision() {
    [[ "${QA_MAILBOX_PROVIDER:-}" == cloudflare ]] || return 0
    : "${QA_MAILBOX_ADDRESS:?QA_MAILBOX_ADDRESS must be the dedicated relay seed address}"
    : "${QA_MAILBOX_API_URL:?QA_MAILBOX_API_URL is required for the Cloudflare QA mailbox relay}"

    local seed_local seed_domain suffix unique_local
    if [[ "$QA_MAILBOX_ADDRESS" =~ ^([A-Za-z0-9][A-Za-z0-9._-]*)@([A-Za-z0-9.-]+)$ ]]; then
        seed_local=${BASH_REMATCH[1]}
        seed_domain=${BASH_REMATCH[2]}
    else
        echo 'QA_MAILBOX_ADDRESS must be an untagged seed email address.' >&2
        return 2
    fi

    suffix="$(date -u +%Y%m%d%H%M%S)-${BASHPID}-${RANDOM}"
    unique_local="${seed_local}+run-${suffix}"
    if ((${#unique_local} > 128)); then
        echo 'The tagged QA mailbox local part exceeds the Worker limit.' >&2
        return 2
    fi
    export QA_MAILBOX_ADDRESS="${unique_local}@${seed_domain}"
}
