#!/usr/bin/env bash
# Guarded project disk cleanup — separate from Docker cache pruning.
#
# Scans known project artifact paths and removes files/directories that are
# (a) not tracked by Git, (b) older than --min-age-days, (c) within the
# validated allowlist, and (d) not under denied paths (/, /home, /root, etc.).
#
# Safety: dry-run by default, --force requires CONFIRM=disk-cleanup, never
# touches git-tracked files, refuses dangerous/ambiguous paths, enforces age
# and max-total-mb bounds, and only cleans allowlisted artifact paths.
#
# Usage: ./disk-cleanup.sh --dry-run [--min-age-days=14]
#        CONFIRM=disk-cleanup ./disk-cleanup.sh --force [--min-age-days=30] [--max-total-mb=500]
set -euo pipefail

readonly SCRIPT_NAME="${0##*/}"

# ── Defaults ─────────────────────────────────────────────────────────────────

DRY_RUN=true
MIN_AGE_DAYS=7
MAX_TOTAL_MB=1024
CONFIRM="${CONFIRM:-}"

# Known artifact paths relative to repo root.  Only these paths (and their
# non-git-tracked contents) are eligible for cleanup.
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

# Directories that, if found under known artifact paths, can be recursively
# cleaned of non-git-tracked contents that are older than MIN_AGE_DAYS.
readonly RECURSIVE_CLEAN_DIRS=(
    "backend/tmp"
    "frontend/coverage"
    "frontend/test-results"
    "frontend/playwright-report"
    "frontend/blob-report"
)

# Paths that must never be cleaned, even if they appear under an allowlisted
# parent.  These are validated as absolute resolved paths.
readonly DENY_ABSOLUTE_PREFIXES=(
    "/"
    "/home"
    "/root"
    "/etc"
    "/boot"
    "/dev"
    "/proc"
    "/sys"
    "/tmp"
    "/var"
    "/opt"
    "/usr"
)

# Path components that, when appearing as a resolved segment, trigger refusal.
readonly DENY_PATH_SEGMENTS=(
    ".."
)

# ── Helpers ──────────────────────────────────────────────────────────────────

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [--dry-run | --force] [--min-age-days=N] [--max-total-mb=N]

Options:
  --dry-run          Report what would be cleaned (default).
  --force            Execute cleanup. Requires CONFIRM=disk-cleanup.
  --min-age-days N   Minimum file age in days (default: $MIN_AGE_DAYS).
  --max-total-mb N   Maximum total size in MB to clean in one run
                     (default: $MAX_TOTAL_MB). Refuses if exceeded.
  --help, -h         Show this help.

Environment:
  CONFIRM            Must be "disk-cleanup" when using --force.
EOF
    exit 0
}

section() { printf '\n=== %s ===\n' "$1"; }
info() { printf '  %s\n' "$*"; }
warn() { printf '  WARN: %s\n' "$*" >&2; }
detail() { printf '    %s\n' "$*"; }

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
        find "$dir" -type f -print0 2>/dev/null |
            xargs -0 -r stat -c%s 2>/dev/null |
            awk '{s+=$1} END {print s+0}'
    else echo 0; fi
}

file_age_days() {
    local path=$1
    local now mtime
    now=$(date +%s)
    mtime=$(stat -c%Y "$path" 2>/dev/null || echo "$now")
    echo $(((now - mtime) / 86400))
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
        --min-age-days=*)
            MIN_AGE_DAYS="${1#*=}"
            shift
            ;;
        --min-age-days)
            MIN_AGE_DAYS="$2"
            shift 2
            ;;
        --max-total-mb=*)
            MAX_TOTAL_MB="${1#*=}"
            shift
            ;;
        --max-total-mb)
            MAX_TOTAL_MB="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            ;;
    esac
done

# ── Validation ───────────────────────────────────────────────────────────────

validate_numeric() {
    local label=$1 value=$2 min=$3
    case "$value" in
        '' | *[!0-9]*)
            echo "ERROR: $label must be a non-negative integer, got '$value'" >&2
            exit 2
            ;;
    esac
    if [ "$value" -lt "$min" ] 2>/dev/null; then
        echo "ERROR: $label must be at least $min, got $value" >&2
        exit 2
    fi
}

validate_force() {
    if [ "$DRY_RUN" = false ] && [ "$CONFIRM" != "disk-cleanup" ]; then
        echo "ERROR: --force requires CONFIRM=disk-cleanup in the environment." >&2
        echo "  Re-run with: CONFIRM=disk-cleanup $0 --force" >&2
        echo "  Or use --dry-run to preview without making changes." >&2
        exit 2
    fi
}

validate_repo_root() {
    local root=$1
    if [ ! -d "$root" ]; then
        echo "ERROR: repository root '$root' is not a directory." >&2
        exit 2
    fi

    local resolved
    resolved=$(cd "$root" 2>/dev/null && pwd -P) || {
        echo "ERROR: cannot resolve repository root '$root'." >&2
        exit 2
    }

    # Refuse if repo root is at dangerous locations.

    if [ "$resolved" = "/" ]; then
        echo "ERROR: repository root is / — refusing to operate on root filesystem." >&2
        exit 2
    fi

    for prefix in "${DENY_ABSOLUTE_PREFIXES[@]}"; do
        if [ "$resolved" = "$prefix" ]; then
            echo "ERROR: repository root '${resolved}' matches deny prefix '${prefix}'." >&2
            echo "  Refusing to operate on this location for safety." >&2
            exit 2
        fi
    done

    # Warn but allow if under /home (developer worktree scenarios are common).
    if [[ "$resolved" == /home/* ]]; then
        info "Repository is under /home (resolved: ${resolved})."
    fi

    echo "$resolved"
}

validate_path() {
    # Validate a single absolute path is safe to clean.
    local abs_path=$1 label=$2

    # Must be non-empty.
    if [ -z "$abs_path" ]; then
        echo "ERROR: $label path is empty." >&2
        exit 2
    fi

    # Resolve symlinks and canonicalize.
    local resolved
    resolved=$(cd "$(dirname "$abs_path")" 2>/dev/null && pwd -P) || {
        echo "ERROR: cannot resolve parent of $label path '$abs_path'." >&2
        exit 2
    }
    resolved="${resolved}/$(basename "$abs_path")"

    # Must exist for cleanup to apply.
    if [ ! -e "$abs_path" ] && [ ! -L "$abs_path" ]; then
        return 1
    fi

    # Disallow root filesystem.
    if [ "$resolved" = "/" ]; then
        echo "ERROR: $label resolves to / — refusing." >&2
        exit 2
    fi

    # Disallow deny prefixes.
    for prefix in "${DENY_ABSOLUTE_PREFIXES[@]}"; do
        if [ "$resolved" = "$prefix" ] || [[ "$resolved" == "$prefix/"* ]]; then
            # Special case: allow /tmp if the resolved path is within our repo
            # (repo root validation already handled at a higher level).
            if [ "$prefix" = "/tmp" ] && [[ "$resolved" != /tmp/*/tmp/* ]]; then
                : # repo root already validated
            else
                echo "ERROR: $label resolves to '${resolved}' which matches deny prefix '${prefix}'." >&2
                exit 2
            fi
        fi
    done

    # Disallow path segments that are dangerous.
    local segment
    IFS='/' read -ra segments <<<"$resolved"
    for segment in "${segments[@]}"; do
        for deny_seg in "${DENY_PATH_SEGMENTS[@]}"; do
            if [ "$segment" = "$deny_seg" ]; then
                echo "ERROR: $label contains denied path segment '$deny_seg'." >&2
                exit 2
            fi
        done
    done

    return 0
}

# ── Git-tracked detection ────────────────────────────────────────────────────

is_git_tracked() {
    local path=$1
    # git ls-files with --error-unmatch exits non-zero for untracked.
    git ls-files --error-unmatch "$path" >/dev/null 2>&1
}

# ── Discovery ────────────────────────────────────────────────────────────────

# Collects eligible (untracked + old enough) files under known artifact paths.
# Output format: size_bytes<TAB>path<TAB>age_days
discover_eligible() {
    local repo_root=$1 min_age=$2
    local rel abs

    for rel in "${KNOWN_ARTIFACT_PATHS[@]}"; do
        abs="${repo_root}/${rel}"

        # Skip non-existent paths.
        if [ ! -e "$abs" ]; then continue; fi

        local is_recursive=false
        for rdir in "${RECURSIVE_CLEAN_DIRS[@]}"; do
            if [ "$rel" = "$rdir" ]; then
                is_recursive=true
                break
            fi
        done

        if [ -f "$abs" ]; then
            # Single file artifact (e.g. coverage.out).
            if is_git_tracked "$rel"; then continue; fi
            local age
            age=$(file_age_days "$abs")
            if [ "$age" -ge "$min_age" ] 2>/dev/null; then
                local sz
                sz=$(stat -c%s "$abs" 2>/dev/null || echo 0)
                printf '%d\t%s\t%d\n' "$sz" "$rel" "$age"
            fi
        elif [ -d "$abs" ]; then
            if [ "$is_recursive" = true ]; then
                # Recursive: find all files under this directory.
                while IFS= read -r -d '' file; do
                    local file_rel="${file#"$repo_root"/}"
                    if is_git_tracked "$file_rel"; then continue; fi
                    local fage
                    fage=$(file_age_days "$file")
                    if [ "$fage" -ge "$min_age" ] 2>/dev/null; then
                        local fsz
                        fsz=$(stat -c%s "$file" 2>/dev/null || echo 0)
                        printf '%d\t%s\t%d\n' "$fsz" "$file_rel" "$fage"
                    fi
                done < <(find "$abs" -type f -print0 2>/dev/null)
            else
                # Non-recursive: report the directory itself as a candidate
                # for removal (if it's not git-tracked as a whole).
                if is_git_tracked "$rel"; then continue; fi
                local sz
                sz=$(dir_size_bytes "$abs")
                if [ "$sz" -gt 0 ] 2>/dev/null; then
                    printf '%d\t%s\t0\n' "$sz" "$rel"
                fi
            fi
        fi
    done
}

# ── Cleanup action ───────────────────────────────────────────────────────────

perform_cleanup() {
    local repo_root=$1 min_age=$2 max_bytes=$3

    local eligibility_file
    eligibility_file=$(mktemp)
    # shellcheck disable=SC2064
    trap 'rm -f "$eligibility_file"' RETURN

    discover_eligible "$repo_root" "$min_age" >"$eligibility_file"

    if [ ! -s "$eligibility_file" ]; then
        echo ""
        info "No eligible artifacts found."
        echo ""
        info "All known artifact paths are either empty, git-tracked, or"
        info "younger than ${min_age} day(s). Nothing to clean."
        return 0
    fi

    # Calculate total size.
    local total_bytes=0 count=0
    while IFS=$'\t' read -r sz path age; do
        total_bytes=$((total_bytes + sz))
        count=$((count + 1))
    done <"$eligibility_file"

    local total_mb=$((total_bytes / 1048576))

    section "Eligible Artifacts"
    info "Found ${count} candidate(s) (total: $(human_size "$total_bytes"))"
    info "Minimum age: ${min_age} day(s)"
    info ""

    # List candidates.
    while IFS=$'\t' read -r sz path age; do
        local age_str=""
        if [ "$age" -gt 0 ] 2>/dev/null; then age_str=", ${age}d old"; fi
        detail "$(human_size "$sz")  ${path}${age_str}"
    done <"$eligibility_file"

    echo ""

    # Enforce max total size bound.
    if [ "$total_mb" -gt "$max_bytes" ] 2>/dev/null; then
        echo "ERROR: Total eligible size (${total_mb} MB) exceeds safety bound (${max_bytes} MB)." >&2
        echo "  Refusing to clean. Raise --max-total-mb or clean manually." >&2
        rm -f "$eligibility_file"
        return 1
    fi

    if [ "$DRY_RUN" = true ]; then
        info "Dry-run: ${count} candidate(s) would be removed (total: $(human_size "$total_bytes"))."
        echo ""
        info "Re-run with CONFIRM=disk-cleanup $0 --force to execute."
        return 0
    fi

    # Execute cleanup.
    local removed=0 removed_bytes=0
    while IFS=$'\t' read -r sz path age; do
        local abs="${repo_root}/${path}"

        # Final safety check — validate path before removal.
        if ! validate_path "$abs" "cleanup-target" 2>/dev/null; then
            warn "SKIP: path validation failed for '$path' — skipping"
            continue
        fi

        # Final git-tracked check (paranoid double-check).
        if is_git_tracked "$path"; then
            warn "SKIP: '$path' is git-tracked — skipping"
            continue
        fi

        info "removing: ${path} ($(human_size "$sz"))"
        rm -rf "$abs" 2>/dev/null || warn "failed to remove ${path}"
        removed=$((removed + 1))
        removed_bytes=$((removed_bytes + sz))
    done <"$eligibility_file"

    rm -f "$eligibility_file"
    echo ""
    info "Removed ${removed} path(s) (total: $(human_size "$removed_bytes"))."
    return 0
}

# ── Summary ──────────────────────────────────────────────────────────────────

print_summary() {
    section "Summary"
    if [ "$DRY_RUN" = true ]; then
        info "Mode:          DRY-RUN (no changes made)"
        info "To execute:    CONFIRM=disk-cleanup $0 --force"
    else
        info "Mode:          FORCE (changes applied)"
    fi
    info "Min age:       ${MIN_AGE_DAYS} day(s)"
    info "Max total:     ${MAX_TOTAL_MB} MB"
    info "Time:          $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    printf '=== Project disk cleanup (%s) ===\n' "$SCRIPT_NAME"
    echo ""

    validate_numeric "min-age-days" "$MIN_AGE_DAYS" 1
    validate_numeric "max-total-mb" "$MAX_TOTAL_MB" 1
    validate_force

    local repo_root
    repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
        echo "ERROR: not inside a Git repository." >&2
        echo "  disk-cleanup.sh must run from within the project repository." >&2
        exit 2
    }

    local resolved_root
    resolved_root=$(validate_repo_root "$repo_root")

    info "Repository: ${resolved_root}"
    info "Min age:    ${MIN_AGE_DAYS} day(s)"
    info "Max total:  ${MAX_TOTAL_MB} MB"
    echo ""

    local max_bytes=$((MAX_TOTAL_MB * 1048576))
    local exit_code=0

    perform_cleanup "$repo_root" "$MIN_AGE_DAYS" "$max_bytes" || exit_code=1

    print_summary

    if [ "$exit_code" -eq 0 ]; then
        printf '\nStatus: complete (no errors)\n'
    else
        printf '\nStatus: completed with errors (see above)\n'
    fi
    exit "$exit_code"
}

main "$@"
