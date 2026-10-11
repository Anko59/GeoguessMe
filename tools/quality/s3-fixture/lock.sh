#!/usr/bin/env bash
# Shared local transition mutex. DIRECTORY is fixed by the calling script.
LOCK_DIRECTORY="$DIRECTORY/.migration-lock"
LOCK_OWNED=false
acquire_migration_lock() {
    umask 077
    [[ ! -L "$ROOT/.local" && ! -L "$DIRECTORY" ]] || return 1
    mkdir -p "$DIRECTORY" || return 1
    mkdir "$LOCK_DIRECTORY" || return 1
    LOCK_OWNED=true
    printf '%s\n' "$$" >"$LOCK_DIRECTORY/owner"
}
release_migration_lock() {
    [[ "$LOCK_OWNED" == true ]] || return 0
    [[ ! -L "$LOCK_DIRECTORY" && ! -L "$LOCK_DIRECTORY/owner" && -f "$LOCK_DIRECTORY/owner" &&
        $(<"$LOCK_DIRECTORY/owner") == "$$" ]] || return 1
    case "$LOCK_DIRECTORY" in
        "$DIRECTORY"/.migration-lock)
            rm -f -- "$LOCK_DIRECTORY/owner" || return 1
            rmdir -- "$LOCK_DIRECTORY" || return 1
            LOCK_OWNED=false
            ;;
        *) return 1 ;;
    esac
}
# Only managed-reader shutdown may inherit the migration holder's ownership;
# creation and ordinary migrations always require their own fresh acquisition.
parent_holds_migration_lock() {
    local parent=${S3_FIXTURE_LOCK_PARENT:-}
    [[ "$parent" =~ ^[1-9][0-9]*$ && ! -L "$LOCK_DIRECTORY" &&
        ! -L "$LOCK_DIRECTORY/owner" && -f "$LOCK_DIRECTORY/owner" &&
        $(<"$LOCK_DIRECTORY/owner") == "$parent" ]] || return 1
    kill -0 "$parent" 2>/dev/null
}
