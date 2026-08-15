#!/usr/bin/env bash

build_e2e_test_args() {
    if [ "$#" -ne 5 ]; then
        echo "build_e2e_test_args requires output, projects, shard, spec, and mode" >&2
        return 2
    fi

    local output_name=$1
    local projects=$2
    local shard=$3
    local spec=$4
    local mode=$5
    local project
    local -a selected_projects
    # shellcheck disable=SC2178 # The nameref intentionally points to an array.
    local -n output=$output_name

    output=(test)
    IFS=',' read -r -a selected_projects <<<"$projects"
    for project in "${selected_projects[@]}"; do
        case "$project" in
            desktop | firefox | mobile) output+=("--project=$project") ;;
            *)
                echo "unsupported Playwright project: $project" >&2
                return 2
                ;;
        esac
    done

    if [ -n "$shard" ]; then
        if [[ ! "$shard" =~ ^([1-9][0-9]*)/([1-9][0-9]*)$ ]] ||
            [ "${BASH_REMATCH[1]:-1}" -gt "${BASH_REMATCH[2]:-0}" ]; then
            echo "GEOGUESSME_E2E_SHARD must be N/M with 1 <= N <= M" >&2
            return 2
        fi
        output+=("--shard=$shard")
    fi

    case "$mode" in
        "") ;;
        --ui)
            if [ -n "$shard" ]; then
                echo "Playwright UI mode cannot be combined with sharding" >&2
                return 2
            fi
            output+=(--ui)
            ;;
        *)
            echo "unsupported E2E runner argument: $mode" >&2
            return 2
            ;;
    esac

    if [ -n "$spec" ]; then
        output+=("$spec")
    fi
}
