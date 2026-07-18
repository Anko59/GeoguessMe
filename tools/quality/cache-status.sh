#!/usr/bin/env bash
# Read-only, deterministic reporting of project-specific Docker resources.
#
# Reports images, builder/build cache usage, project volumes, and workspace
# artifacts for the GeoGuessMe project.  Output is bounded, avoids host-wide
# destructive operations, filters by PROJECT_PREFIX (default: geoguessme), and
# is suitable for CI/docs.
set -euo pipefail

readonly SCRIPT_NAME="${0##*/}"
readonly PROJECT_PREFIX="${PROJECT_PREFIX:-geoguessme}"
readonly COMPOSE_PROJECT="${COMPOSE_PROJECT:-${PROJECT_PREFIX}}"

# ── Helpers ──────────────────────────────────────────────────────────────────

section() {
    printf '\n=== %s ===\n' "$1"
}

docker_available() {
    command -v docker >/dev/null 2>&1
}

human_size() {
    local bytes=$1
    if [ "$bytes" -lt 1024 ]; then
        printf '%dB' "$bytes"
    elif [ "$bytes" -lt 1048576 ]; then
        printf '%dKB' "$((bytes / 1024))"
    elif [ "$bytes" -lt 1073741824 ]; then
        printf '%dMB' "$((bytes / 1048576))"
    else
        printf '%dGB' "$((bytes / 1073741824))"
    fi
}

# Accumulate directory sizes in bytes without sudo or bc.
dir_size_bytes() {
    local dir=$1
    if [ -d "$dir" ]; then
        find "$dir" -type f -exec stat -c%s {} + 2>/dev/null | tr '\n' '+' | sed 's/+$//' | awk '{s=0; split($0,a,"+"); for(i in a) s+=a[i]; print s}' 2>/dev/null || echo 0
    else
        echo 0
    fi
}

# ── Docker Images ────────────────────────────────────────────────────────────

report_images() {
    section "Docker Images (prefix: ${PROJECT_PREFIX})"
    if ! docker_available; then
        printf '  SKIP: Docker unavailable\n'
        return
    fi

    local images
    images=$(docker images --format '{{.Repository}}:{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}' 2>/dev/null | grep -i "${PROJECT_PREFIX}" || true)

    if [ -z "$images" ]; then
        printf '  (no matching images found)\n'
        printf '  Total: 0 images\n'
        return
    fi

    local count=0
    while IFS=$'\t' read -r repo size_str created; do
        printf '  %-50s %-10s %s\n' "$repo" "$size_str" "$created"
        count=$((count + 1))
    done <<<"$images"

    printf '  Total: %d images\n' "$count"
}

# ── Docker Build Cache ───────────────────────────────────────────────────────

report_build_cache() {
    section "Docker Build Cache"
    if ! docker_available; then
        printf '  SKIP: Docker unavailable\n'
        return
    fi

    # docker system df is the most portable cache reporter.
    local df_output
    df_output=$(docker system df --format '{{.Type}}\t{{.Size}}\t{{.Reclaimable}}' 2>/dev/null || true)
    if [ -z "$df_output" ]; then
        printf '  (build cache data unavailable)\n'
        return
    fi

    local found=0
    while IFS=$'\t' read -r type size reclaimable; do
        if [ "$type" = "Build Cache" ]; then
            printf '  Disk usage: %s (reclaimable: %s)\n' "$size" "$reclaimable"
            found=1
        fi
    done <<<"$df_output"
    if [ "$found" -eq 0 ]; then
        printf '  (no build cache data)\n'
    fi

    # Also report total disk usage for context.
    while IFS=$'\t' read -r type size reclaimable; do
        if [ "$type" = "Total" ]; then
            printf '  Total Docker disk usage: %s (reclaimable: %s)\n' "$size" "$reclaimable"
        fi
    done <<<"$df_output"
}

# ── Docker Volumes ───────────────────────────────────────────────────────────

report_volumes() {
    section "Docker Volumes (project: ${COMPOSE_PROJECT})"
    if ! docker_available; then
        printf '  SKIP: Docker unavailable\n'
        return
    fi

    # Try compose project first, fall back to prefix match.
    local volumes
    volumes=$(docker volume ls --format '{{.Name}}' 2>/dev/null | grep -i "${COMPOSE_PROJECT}" | head -50 || true)

    if [ -z "$volumes" ]; then
        volumes=$(docker volume ls --format '{{.Name}}' 2>/dev/null | grep -i "${PROJECT_PREFIX}" | head -50 || true)
    fi

    if [ -z "$volumes" ]; then
        printf '  (no matching volumes found)\n'
        printf '  Total: 0 volumes\n'
        return
    fi

    local count=0
    while IFS= read -r name; do
        [ -n "$name" ] || continue
        local mount_point
        mount_point=$(docker volume inspect --format '{{.Mountpoint}}' "$name" 2>/dev/null || true)
        local size_str=""
        if [ -n "$mount_point" ] && [ -d "$mount_point" ]; then
            local size_bytes
            size_bytes=$(dir_size_bytes "$mount_point")
            if [ "$size_bytes" -gt 0 ] 2>/dev/null; then
                size_str="$(human_size "$size_bytes")"
            fi
        fi
        printf '  %-50s %s\n' "$name" "${size_str:-(no data)}"
        count=$((count + 1))
    done <<<"$volumes"

    printf '  Total: %d volumes\n' "$count"
}

# ── Workspace Artifacts ──────────────────────────────────────────────────────

report_artifacts() {
    section "Workspace Artifacts"
    local repo_root
    repo_root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)

    local artifacts_found=0

    # Backend binary
    if [ -f "${repo_root}/backend/bin/geoguessme" ]; then
        local size
        size=$(stat -c%s "${repo_root}/backend/bin/geoguessme" 2>/dev/null || echo 0)
        printf '  %-55s %s\n' 'backend/bin/geoguessme' "$(human_size "$size")"
        artifacts_found=$((artifacts_found + 1))
    fi

    # Frontend dist
    if [ -d "${repo_root}/frontend/dist" ]; then
        local size
        size=$(dir_size_bytes "${repo_root}/frontend/dist")
        if [ "$size" -gt 0 ]; then
            printf '  %-55s %s\n' 'frontend/dist/' "$(human_size "$size")"
            artifacts_found=$((artifacts_found + 1))
        fi
    fi

    # Coverage reports (Go)
    if [ -f "${repo_root}/backend/coverage.out" ]; then
        local size
        size=$(stat -c%s "${repo_root}/backend/coverage.out" 2>/dev/null || echo 0)
        printf '  %-55s %s\n' 'backend/coverage.out' "$(human_size "$size")"
        artifacts_found=$((artifacts_found + 1))
    fi

    if [ -d "${repo_root}/frontend/coverage" ]; then
        local size
        size=$(dir_size_bytes "${repo_root}/frontend/coverage")
        if [ "$size" -gt 0 ]; then
            printf '  %-55s %s\n' 'frontend/coverage/' "$(human_size "$size")"
            artifacts_found=$((artifacts_found + 1))
        fi
    fi

    # Playwright test results
    if [ -d "${repo_root}/frontend/test-results" ]; then
        local size
        size=$(dir_size_bytes "${repo_root}/frontend/test-results")
        if [ "$size" -gt 0 ]; then
            printf '  %-55s %s\n' 'frontend/test-results/' "$(human_size "$size")"
            artifacts_found=$((artifacts_found + 1))
        fi
    fi

    if [ -d "${repo_root}/frontend/playwright-report" ]; then
        local size
        size=$(dir_size_bytes "${repo_root}/frontend/playwright-report")
        if [ "$size" -gt 0 ]; then
            printf '  %-55s %s\n' 'frontend/playwright-report/' "$(human_size "$size")"
            artifacts_found=$((artifacts_found + 1))
        fi
    fi

    if [ "$artifacts_found" -eq 0 ]; then
        printf '  (no build artifacts found)\n'
    fi
}

# ── Summary ──────────────────────────────────────────────────────────────────

print_summary() {
    section "Summary"
    if ! docker_available; then
        printf '  Docker is not available on this host.\n'
        printf '  Install Docker to include image, cache, and volume reporting.\n'
    else
        local docker_version
        docker_version=$(docker --version 2>/dev/null || echo "unknown")
        printf '  Docker:  %s\n' "$docker_version"
        local image_count volume_count
        image_count=$(docker images --format '{{.Repository}}' 2>/dev/null | grep -ci "${PROJECT_PREFIX}" || echo 0)
        volume_count=$(docker volume ls --format '{{.Name}}' 2>/dev/null | grep -ci -e "${COMPOSE_PROJECT}" -e "${PROJECT_PREFIX}" || echo 0)
        printf '  Images:  %d matching "%s"\n' "$image_count" "$PROJECT_PREFIX"
        printf '  Volumes: %d matching "%s"\n' "$volume_count" "$PROJECT_PREFIX"
    fi
    printf '  Time:    %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    printf '=== %s cache status (%s) ===\n' "$PROJECT_PREFIX" "$SCRIPT_NAME"

    report_images
    report_build_cache
    report_volumes
    report_artifacts
    print_summary

    printf '\nStatus: complete (read-only, no modifications made)\n'
}

main "$@"
