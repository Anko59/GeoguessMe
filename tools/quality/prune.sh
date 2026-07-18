#!/usr/bin/env bash
# Bounded, project-scoped Docker artifact and cache pruning.
#
# Prunes Docker images, build cache, volumes, and workspace artifacts that
# belong to a specific project (PROJECT_PREFIX).  Every destructive action is
# gated behind --force; the default is a read-only dry-run report.
#
# Safety guarantees:
#   - Dry-run by default; --force required for any mutation
#   - Refuses to run when PROJECT_PREFIX is missing or ambiguous
#   - Never prunes host-wide resources (images/volumes filtered by prefix)
#   - Never deletes arbitrary filesystem paths
#   - Enforces documented MAX_* bounds; refuses if count exceeds limit
#   - Volumes and build cache are opt-in (--include-volumes, --include-build-cache)
#
# Usage:
#   PROJECT_PREFIX=geoguessme ./prune.sh --dry-run
#   PROJECT_PREFIX=geoguessme ./prune.sh --force
#   PROJECT_PREFIX=geoguessme ./prune.sh --force --include-volumes --include-build-cache
#
# Documented bounds (override with CLI flags):
#   --max-images    Maximum project images to prune in one run (default: 50)
#   --max-volumes   Maximum project volumes to prune in one run (default: 20)
set -euo pipefail

readonly SCRIPT_NAME="${0##*/}"

# ── Defaults ─────────────────────────────────────────────────────────────────

DRY_RUN=true
INCLUDE_VOLUMES=false
INCLUDE_BUILD_CACHE=false
MAX_IMAGES=50
MAX_VOLUMES=20
PROJECT_PREFIX="${PROJECT_PREFIX:-}"

# Known artifact paths relative to repo root.
readonly KNOWN_ARTIFACT_PATHS=(
    "backend/bin"
    "backend/tmp"
    "backend/coverage.out"
    "frontend/dist"
    "frontend/coverage"
    "frontend/test-results"
    "frontend/playwright-report"
    "frontend/blob-report"
)

# Prefixes that are too generic to be a safe project scope.
readonly AMBIGUOUS_PREFIXES=(
    "docker" "sha256" "library" "registry" "localhost"
    "compose" "none" "host" "default" "all" "global"
    "scratch" "busybox" "alpine" "ubuntu" "debian" "centos"
)

# ── Helpers ──────────────────────────────────────────────────────────────────

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [--dry-run | --force] [--include-volumes] [--include-build-cache]
                    [--max-images=N] [--max-volumes=N]

Options:
  --dry-run             Report what would be pruned (default).
  --force               Execute pruning. Requires CONFIRM=prune in environment.
  --include-volumes     Also prune project Docker volumes (opt-in).
  --include-build-cache Also prune dangling Docker build cache (opt-in).
  --max-images N        Maximum project images to prune (default: $MAX_IMAGES).
  --max-volumes N       Maximum project volumes to prune (default: $MAX_VOLUMES).

Environment:
  PROJECT_PREFIX        Required. Docker resource name prefix (e.g. geoguessme).
  CONFIRM               Must be "prune" when using --force.
EOF
    exit 0
}

section() { printf '\n=== %s ===\n' "$1"; }
info() { printf '  %s\n' "$*"; }
warn() { printf '  WARN: %s\n' "$*" >&2; }
detail() { printf '    %s\n' "$*"; }

docker_available() { command -v docker >/dev/null 2>&1; }

human_size() {
    local bytes=$1
    if [ "$bytes" -lt 1024 ]; then
        printf '%dB' "$bytes"
    elif [ "$bytes" -lt 1048576 ]; then
        printf '%dKB' "$((bytes / 1024))"
    elif [ "$bytes" -lt 1073741824 ]; then
        printf '%dMB' "$((bytes / 1048576))"
    else printf '%dGB' "$((bytes / 1073741824))"; fi
}

dir_size_bytes() {
    local dir=$1
    if [ -d "$dir" ]; then
        find "$dir" -type f -exec stat -c%s {} + 2>/dev/null |
            tr '\n' '+' | sed 's/+$//' |
            awk '{s=0; split($0,a,"+"); for(i in a) s+=a[i]; print s}' 2>/dev/null || echo 0
    else echo 0; fi
}

# ── Argument parsing ─────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
    case "$1" in
        --help | -h) usage ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force)
            DRY_RUN=false
            shift
            ;;
        --include-volumes)
            INCLUDE_VOLUMES=true
            shift
            ;;
        --include-build-cache)
            INCLUDE_BUILD_CACHE=true
            shift
            ;;
        --max-images=*)
            MAX_IMAGES="${1#*=}"
            shift
            ;;
        --max-images)
            MAX_IMAGES="$2"
            shift 2
            ;;
        --max-volumes=*)
            MAX_VOLUMES="${1#*=}"
            shift
            ;;
        --max-volumes)
            MAX_VOLUMES="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            ;;
    esac
done

# ── Scope validation ─────────────────────────────────────────────────────────

validate_scope() {
    if [ -z "$PROJECT_PREFIX" ]; then
        echo "ERROR: PROJECT_PREFIX is not set." >&2
        echo "  Set it to a project-specific prefix, e.g. PROJECT_PREFIX=geoguessme" >&2
        echo "  This guards against accidental host-wide pruning." >&2
        exit 2
    fi

    if [ "${#PROJECT_PREFIX}" -lt 3 ]; then
        echo "ERROR: PROJECT_PREFIX='${PROJECT_PREFIX}' is too short (< 3 characters)." >&2
        echo "  A short prefix risks matching unrelated Docker resources." >&2
        exit 2
    fi

    local lower
    lower=$(echo "$PROJECT_PREFIX" | tr '[:upper:]' '[:lower:]')
    for amb in "${AMBIGUOUS_PREFIXES[@]}"; do
        if [ "$lower" = "$amb" ]; then
            echo "ERROR: PROJECT_PREFIX='${PROJECT_PREFIX}' is ambiguous." >&2
            echo "  '${PROJECT_PREFIX}' is a generic Docker/OS term that may match" >&2
            echo "  resources outside the project. Use a project-specific prefix." >&2
            exit 2
        fi
    done

    echo "Scope: PROJECT_PREFIX=${PROJECT_PREFIX}"
}

validate_force() {
    if [ "$DRY_RUN" = false ] && [ "${CONFIRM:-}" != "prune" ]; then
        echo "ERROR: --force requires CONFIRM=prune in the environment." >&2
        echo "  Re-run with: CONFIRM=prune $0 --force" >&2
        echo "  Or use --dry-run to preview without making changes." >&2
        exit 2
    fi
}

# ── Bounds enforcement (pre-count) ───────────────────────────────────────────

enforce_bound() {
    local label="$1" observed="$2" limit="$3"
    if [ "$observed" -gt "$limit" ] 2>/dev/null; then
        echo "ERROR: ${label} count (${observed}) exceeds safety bound (${limit})." >&2
        echo "  Refusing to prune. Raise the bound with --max-${label}=N or" >&2
        echo "  prune resources individually." >&2
        return 1
    fi
    return 0
}

# ── Docker Images ────────────────────────────────────────────────────────────

prune_images() {
    section "Docker Images (prefix: ${PROJECT_PREFIX})"
    if ! docker_available; then
        info "SKIP: Docker unavailable"
        return 0
    fi

    local images
    images=$(docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null |
        grep -i "${PROJECT_PREFIX}" || true)

    if [ -z "$images" ]; then
        info "No project images found."
        return 0
    fi

    local count
    count=$(echo "$images" | wc -l)
    info "Found ${count} project image(s)"

    if ! enforce_bound "images" "$count" "$MAX_IMAGES"; then return 1; fi

    local pruned=0
    while IFS= read -r img; do
        [ -n "$img" ] || continue
        if [ "$DRY_RUN" = true ]; then
            detail "would remove: ${img}"
        else
            info "removing: ${img}"
            docker rmi "$img" 2>/dev/null || warn "failed to remove ${img}"
        fi
        pruned=$((pruned + 1))
    done <<<"$images"

    if [ "$DRY_RUN" = true ]; then
        info "Dry-run: ${pruned} image(s) would be removed."
    else
        info "Pruned ${pruned} image(s)."
    fi
    return 0
}

# ── Docker Build Cache ───────────────────────────────────────────────────────

prune_build_cache() {
    section "Docker Build Cache"
    if ! docker_available; then
        info "SKIP: Docker unavailable"
        return 0
    fi

    # Report current state.
    local df_output
    df_output=$(docker system df --format '{{.Type}}\t{{.Size}}\t{{.Reclaimable}}' 2>/dev/null || true)

    local build_size="" build_reclaimable=""
    while IFS=$'\t' read -r type size reclaimable; do
        if [ "$type" = "Build Cache" ]; then
            build_size="$size"
            build_reclaimable="$reclaimable"
        fi
    done <<<"$df_output"

    if [ -z "$build_size" ]; then
        info "No build cache data available."
        return 0
    fi

    info "Build cache usage: ${build_size} (reclaimable: ${build_reclaimable})"

    # Only prune dangling build cache (not all unused).  This is the safer
    # default: --force without --all removes only dangling layers that no
    # image references.  There is no Docker-blessed project-namespaced
    # builder prune filter, but dangling-only is inherently bounded.
    if [ "$DRY_RUN" = true ]; then
        info "Dry-run: dangling build cache would be pruned (docker builder prune --force)"
    else
        info "Pruning dangling build cache..."
        docker builder prune --force 2>/dev/null || warn "builder prune returned non-zero"
        info "Build cache pruned."
    fi
    return 0
}

# ── Docker Volumes ───────────────────────────────────────────────────────────

prune_volumes() {
    section "Docker Volumes (prefix: ${PROJECT_PREFIX})"
    if ! docker_available; then
        info "SKIP: Docker unavailable"
        return 0
    fi

    local volumes
    volumes=$(docker volume ls --format '{{.Name}}' 2>/dev/null |
        grep -i "${PROJECT_PREFIX}" || true)

    if [ -z "$volumes" ]; then
        info "No project volumes found."
        return 0
    fi

    local count
    count=$(echo "$volumes" | wc -l)
    info "Found ${count} project volume(s)"

    if ! enforce_bound "volumes" "$count" "$MAX_VOLUMES"; then return 1; fi

    local pruned=0
    while IFS= read -r vol; do
        [ -n "$vol" ] || continue

        local mount_point size_str=""
        mount_point=$(docker volume inspect --format '{{.Mountpoint}}' "$vol" 2>/dev/null || true)
        if [ -n "$mount_point" ] && [ -d "$mount_point" ]; then
            local sz
            sz=$(dir_size_bytes "$mount_point")
            if [ "$sz" -gt 0 ] 2>/dev/null; then size_str=" ($(human_size "$sz"))"; fi
        fi

        if [ "$DRY_RUN" = true ]; then
            detail "would remove: ${vol}${size_str}"
        else
            info "removing: ${vol}${size_str}"
            docker volume rm "$vol" 2>/dev/null || warn "failed to remove volume ${vol}"
        fi
        pruned=$((pruned + 1))
    done <<<"$volumes"

    if [ "$DRY_RUN" = true ]; then
        info "Dry-run: ${pruned} volume(s) would be removed."
    else
        info "Pruned ${pruned} volume(s)."
    fi
    return 0
}

# ── Workspace Artifacts ──────────────────────────────────────────────────────

prune_artifacts() {
    section "Workspace Artifacts"
    local repo_root
    repo_root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)

    local found=0 total_bytes=0 pruned=0

    # Collect existing artifacts first.
    declare -a existing=()
    for rel in "${KNOWN_ARTIFACT_PATHS[@]}"; do
        local abs="${repo_root}/${rel}"
        if [ -e "$abs" ]; then
            local sz=0
            if [ -f "$abs" ]; then sz=$(stat -c%s "$abs" 2>/dev/null || echo 0); fi
            if [ -d "$abs" ]; then sz=$(dir_size_bytes "$abs"); fi
            total_bytes=$((total_bytes + sz))
            existing+=("$rel")
            found=$((found + 1))
        fi
    done

    if [ "$found" -eq 0 ]; then
        info "No workspace artifacts found."
        return 0
    fi

    info "Found ${found} artifact path(s) (total: $(human_size "$total_bytes"))"

    for rel in "${existing[@]}"; do
        local abs="${repo_root}/${rel}"
        local sz=0
        if [ -f "$abs" ]; then sz=$(stat -c%s "$abs" 2>/dev/null || echo 0); fi
        if [ -d "$abs" ]; then sz=$(dir_size_bytes "$abs"); fi
        if [ "$DRY_RUN" = true ]; then
            detail "would remove: ${rel} ($(human_size "$sz"))"
        else
            info "removing: ${rel} ($(human_size "$sz"))"
            rm -rf "$abs"
        fi
        pruned=$((pruned + 1))
    done

    if [ "$DRY_RUN" = true ]; then
        info "Dry-run: ${pruned} artifact path(s) would be removed."
    else
        info "Pruned ${pruned} artifact path(s)."
    fi
    return 0
}

# ── Summary ──────────────────────────────────────────────────────────────────

print_summary() {
    section "Summary"
    if [ "$DRY_RUN" = true ]; then
        info "Mode:       DRY-RUN (no changes made)"
        info "Re-run with CONFIRM=prune $0 --force to execute."
    else
        info "Mode:       FORCE (changes applied)"
    fi
    info "Scope:      PROJECT_PREFIX=${PROJECT_PREFIX}"
    info "Images:     max ${MAX_IMAGES}"
    info "Volumes:    max ${MAX_VOLUMES} (included: ${INCLUDE_VOLUMES})"
    info "Build cache: included: ${INCLUDE_BUILD_CACHE}"
    info "Time:       $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    printf '=== %s prune (%s) ===\n' "$PROJECT_PREFIX" "$SCRIPT_NAME"
    echo ""

    validate_scope
    validate_force

    local exit_code=0

    prune_images || exit_code=1
    if [ "$INCLUDE_BUILD_CACHE" = true ]; then prune_build_cache || exit_code=1; fi
    if [ "$INCLUDE_VOLUMES" = true ]; then prune_volumes || exit_code=1; fi
    prune_artifacts || exit_code=1

    print_summary

    if [ "$exit_code" -eq 0 ]; then
        printf '\nStatus: complete (no errors)\n'
    else
        printf '\nStatus: completed with errors (see above)\n'
    fi
    exit "$exit_code"
}

main "$@"
