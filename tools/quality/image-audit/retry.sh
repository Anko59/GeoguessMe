#!/usr/bin/env bash
# Transport retries only: vulnerability/policy results never become retryable.
# Sourced by audit.sh; RETRY_KIND describes the last failed operation.

retryable_log() {
    local log=$1
    # An error chain can contain a timeout alongside a definitive denial.
    # Denials, invalid content/signatures and missing artifacts always win.
    if grep -Eiq 'unauthorized|authentication required|denied|forbidden|\b(401|403|404)\b|manifest unknown|not found|no such|signature|digest mismatch|certificate|x509|invalid argument|unknown flag' "$log"; then
        return 1
    fi
    grep -Eiq '\b429\b|too many requests|toomanyrequests|\b(500|502|503|504)\b|connection (reset|refused)|network is unreachable|temporary failure|i/o timeout|TLS handshake timeout|context deadline exceeded|unexpected EOF|connection timed out|dial tcp.*timeout' "$log"
}

retry_delay() {
    local attempt=$1 log=$2 delay header deadline wait_seconds now
    delay=$((2 ** (attempt - 1) + RANDOM % 3))
    # Docker/Trivy do not always expose headers. Honor delta-seconds or HTTP
    # dates when logged; an unparseable advertised delay fails conservatively.
    header=$(sed -nE 's/.*[Rr]etry-[Aa]fter:[[:space:]]*(.*)/\1/p' "$log" | tail -1)
    if [ -n "$header" ]; then
        if [[ "$header" =~ ^[0-9]{1,6}([[:space:]]|$) ]]; then
            header=${BASH_REMATCH[0]//[[:space:]]/}
            wait_seconds=$((10#$header))
        else
            deadline=$(date -u -d "$header" +%s 2>/dev/null || date -j -u -f '%a, %d %b %Y %H:%M:%S GMT' "$header" +%s 2>/dev/null) || return 1
            now=$(date -u +%s)
            wait_seconds=$((deadline - now))
        fi
        if ((wait_seconds > delay)); then delay=$wait_seconds; fi
    fi
    # The whole retry budget is bounded. A longer server request ends the
    # operation rather than violating its advertised backoff.
    if ((delay > 60)); then
        return 1
    fi
    printf '%s\n' "$delay"
}

run_with_retry() {
    local label=$1 log=$2 attempt rc delay
    shift 2
    RETRY_KIND=operation
    : >"$log"
    for attempt in 1 2 3 4; do
        if "$@" >"$log.attempt" 2>&1; then
            cat "$log.attempt" >>"$log"
            cat "$log.attempt"
            return 0
        else
            rc=$?
        fi
        cat "$log.attempt" >>"$log"
        cat "$log.attempt" >&2
        # 42 is reserved by our Trivy blocking pass for genuine findings.
        if ((rc == 42)) || ! retryable_log "$log.attempt"; then
            return "$rc"
        fi
        # Public result consumed by callers sourcing this helper.
        # shellcheck disable=SC2034
        RETRY_KIND=transient
        if ((attempt == 4)) || ! delay=$(retry_delay "$attempt" "$log.attempt"); then
            echo "image-audit: exhausted transient failure: $label" >&2
            return "$rc"
        fi
        echo "image-audit: transient failure: $label; retry $((attempt + 1))/4 after ${delay}s" >&2
        sleep "$delay"
    done
}

if [[ "${BASH_SOURCE[0]}" = "$0" ]]; then
    [ $# -ge 3 ] || {
        echo 'usage: retry.sh LABEL LOG COMMAND [ARGS...]' >&2
        exit 2
    }
    run_with_retry "$@"
fi
