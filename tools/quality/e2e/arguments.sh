#!/usr/bin/env bash

build_e2e_test_args() {
    if [ "$#" -ne 4 ]; then
        echo "build_e2e_test_args requires projects, shard, spec, and mode" >&2
        return 2
    fi

    local projects=$1
    local shard=$2
    local spec=$3
    local mode=$4
    local project
    local -a selected_projects

    E2E_TEST_ARGS=(test)
    IFS=',' read -r -a selected_projects <<<"$projects"
    for project in "${selected_projects[@]}"; do
        case "$project" in
            desktop | firefox | mobile) E2E_TEST_ARGS+=("--project=$project") ;;
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
        E2E_TEST_ARGS+=("--shard=$shard")
    fi

    case "$mode" in
        "") ;;
        --ui)
            if [ -n "$shard" ]; then
                echo "Playwright UI mode cannot be combined with sharding" >&2
                return 2
            fi
            E2E_TEST_ARGS+=(--ui)
            ;;
        *)
            echo "unsupported E2E runner argument: $mode" >&2
            return 2
            ;;
    esac

    if [ -n "$spec" ]; then
        E2E_TEST_ARGS+=("$spec")
    fi
}
