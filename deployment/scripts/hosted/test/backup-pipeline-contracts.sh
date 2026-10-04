#!/bin/bash
set -euo pipefail

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
TMP=$(mktemp -d)
# Only this test-created temporary tree is removed.
trap 'rm -rf -- "$TMP"' EXIT INT TERM
export REAL_GZIP
REAL_GZIP=$(command -v gzip)

fail() {
    printf 'backup pipeline contract failed: %s\n' "$*" >&2
    exit 1
}

mkdir -p "$TMP/scripts"
cp "$ROOT/deployment/scripts/hosted/backup.sh" \
    "$ROOT/deployment/scripts/hosted/restore-rehearsal.sh" "$TMP/scripts/"
# Mock shared helpers and all backends, but retain real shell pipelines/gzip.
cat >"$TMP/scripts/common.sh" <<'COMMON'
APP_ROOT="$CASE/app"
STATE_ROOT="$CASE/state"
LOCK_ROOT="$CASE/locks"
validate_environment() { :; }
require_secret_file() { :; }
identity_env_file() { printf '%s/identity.env\n' "$CASE"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
flock() { :; }
mktemp() {
    if [ "$MODE" = restore-tempfail ]; then
        if [ -f "$CASE/first-temp" ]; then
            printf 'mktemp-rejected\n' >>"$CASE/events"
            return 51
        fi
        command mktemp "$@" || return
        touch "$CASE/first-temp"
    else
        command mktemp "$@"
    fi
}
compose() {
    printf 'app-dump\n' >>"$CASE/events"
    case "$MODE" in
        dump-first) return 41 ;;
        signature) die 'database image signature rejected' ;;
        dump-partial) printf 'CREATE TABLE unfinished ('; return 42 ;;
    esac
    printf 'SELECT 1;\n'
}
compose_identity() {
    printf 'identity-dump\n' >>"$CASE/events"
    case "$MODE" in
        identity-first) return 43 ;;
        identity-signature) die 'identity image signature rejected' ;;
        identity-partial) printf 'CREATE TABLE unfinished ('; return 44 ;;
    esac
    printf 'SELECT 2;\n'
}
gzip() {
    case "$MODE:$1" in
        gzip-failure:-9)
            "$REAL_GZIP" "$@"
            return 45
            ;;
        gzip-check:-t) return 46 ;;
        restore-gzip:-dc)
            "$REAL_GZIP" "$@"
            return 47
            ;;
        restore-identity-gzip:-dc)
            "$REAL_GZIP" "$@"
            [ ! -f "$CASE/identity-downloaded" ] || return 48
            return 0
            ;;
    esac
    "$REAL_GZIP" "$@"
}
restic() {
    shift
    printf 'restic %s\n' "$*" >>"$CASE/events"
    case "$1" in
        snapshots) printf '[{"short_id":"abc123"}]\n' ;;
        ls)
            printf '{"path":"/backup/postgres-fixture.sql.gz"}\n'
            printf '{"path":"/backup/keycloak-fixture.sql.gz"}\n'
            ;;
        dump)
            case "$3" in
                /backup/postgres-*) cat "$CASE/app.sql.gz" ;;
                /backup/keycloak-*)
                    touch "$CASE/identity-downloaded"
                    cat "$CASE/identity.sql.gz"
                    ;;
                *) die 'unexpected restore path' ;;
            esac
            ;;
        backup)
            shift
            for path in "$@"; do
                case "$path" in
                    /backup/*)
                        "$REAL_GZIP" -dc "$STATE_ROOT/backups/$ENVIRONMENT/${path##*/}" \
                            >>"$CASE/uploaded.sql"
                        ;;
                esac
            done
            ;;
        forget | init) ;;
        *) die 'unexpected restic invocation' ;;
    esac
}
select_postgres_image() { printf 'app-selected-image\n'; }
select_identity_postgres_image() { printf 'identity-selected-image\n'; }
docker() {
    printf 'docker %s\n' "$*" >>"$CASE/events"
    case "$1" in
        run | rm) ;;
        exec)
            case " $* " in
                *' pg_isready '*) return 0 ;;
                *' -c '*) printf 'probe\n' >>"$CASE/probes" ;;
                *)
                    cat >>"$CASE/restored.sql"
                    case "$MODE" in
                        restore-psql) return 49 ;;
                        restore-identity-psql)
                            [ ! -f "$CASE/identity-downloaded" ] || return 50
                            ;;
                    esac
                    ;;
            esac
            ;;
        *) die 'unexpected Docker invocation' ;;
    esac
}
COMMON

new_case() {
    export CASE MODE ENVIRONMENT TMPDIR
    MODE=$1 ENVIRONMENT=$2 CASE="$TMP/$1-$2"
    TMPDIR="$CASE/tmp"
    mkdir -p "$CASE/app/$ENVIRONMENT/current" "$CASE/state/backups/$ENVIRONMENT" \
        "$CASE/locks" "$TMPDIR"
    touch "$CASE/events" "$CASE/identity.env"
}

assert_no_temps() {
    local path
    for path in "$CASE/state/backups/$ENVIRONMENT/"*.sql.gz "$TMPDIR/"*; do
        [ ! -e "$path" ] || fail "$MODE left temporary data: $path"
    done
}

run_script() {
    local script=$1 expected=$2 actual=0
    # Execute the production shebang, not sh or an errexit-disabled function.
    "$TMP/scripts/$script" "$ENVIRONMENT" >"$CASE/output" 2>&1 || actual=$?
    if [ "$actual" -ne "$expected" ]; then
        cat "$CASE/output" >&2
        fail "$MODE: expected exit $expected, got $actual"
    fi
    assert_no_temps
}

backup_failure() {
    local mode=$1 status=$2
    new_case "$mode" production
    printf 'previous-success\n' >"$CASE/state/backups/production/last-success"
    run_script backup.sh "$status"
    ! grep -q '^restic ' "$CASE/events" || fail "$mode reached Restic"
    [ "$(<"$CASE/state/backups/production/last-success")" = previous-success ] ||
        fail "$mode replaced last-success"
    ! grep -q 'backup completed:' "$CASE/output" || fail "$mode reported success"
}

backup_failure dump-first 41
backup_failure dump-partial 42
backup_failure signature 1
backup_failure identity-first 43
backup_failure identity-partial 44
backup_failure identity-signature 1
backup_failure gzip-failure 45
backup_failure gzip-check 46

# A failed first backup must not create a fresh success marker either.
new_case dump-first dev
run_script backup.sh 41
[ ! -e "$CASE/state/backups/dev/last-success" ] || fail 'failed dump created last-success'
! grep -q '^restic ' "$CASE/events" || fail 'failed dev dump reached Restic'

for environment in dev production; do
    new_case success "$environment"
    run_script backup.sh 0
    [ -s "$CASE/state/backups/$environment/last-success" ] || fail 'success marker missing'
    [ "$(stat -c '%a' "$CASE/state/backups/$environment/last-success")" = 600 ] ||
        fail 'success marker permissions changed'
    grep -q '^restic backup ' "$CASE/events" || fail 'successful dumps were not uploaded'
    grep -q '^restic forget ' "$CASE/events" || fail 'retention was not applied'
    grep -q 'backup completed:' "$CASE/output" || fail 'successful backup was not reported'
    expected='SELECT 1;'
    if [ "$environment" = production ]; then expected=$'SELECT 1;\nSELECT 2;'; fi
    [ "$(<"$CASE/uploaded.sql")" = "$expected" ] || fail 'uploaded SQL payload changed'
done

restore_case() {
    local mode=$1 status=$2 probes=$3 environment=${4:-production}
    new_case "$mode" "$environment"
    printf 'SELECT 1;\n' | "$REAL_GZIP" -9 >"$CASE/app.sql.gz"
    printf 'SELECT 2;\n' | "$REAL_GZIP" -9 >"$CASE/identity.sql.gz"
    run_script restore-rehearsal.sh "$status"
    grep -q '^docker rm -f geoguessme-restore-' "$CASE/events" ||
        fail "$mode did not clean up rehearsal containers"
    grep -q 'app-selected-image' "$CASE/events" || fail 'restore bypassed app image selection'
    local actual_probes=0
    if [ -f "$CASE/probes" ]; then actual_probes=$(wc -l <"$CASE/probes"); fi
    [ "$actual_probes" -eq "$probes" ] || fail "$mode continued after pipeline failure"
    if [ "$status" -ne 0 ]; then
        ! grep -q 'restore rehearsal passed:' "$CASE/output" || fail "$mode reported success"
    else
        grep -q 'restore rehearsal passed:' "$CASE/output" || fail 'restore success missing'
        local expected='SELECT 1;'
        if [ "$environment" = production ]; then
            grep -q 'identity-selected-image' "$CASE/events" || fail 'restore bypassed identity image selection'
            expected=$'SELECT 1;\nSELECT 2;'
        fi
        [ "$(<"$CASE/restored.sql")" = "$expected" ] || fail 'restored SQL payload changed'
    fi
}

# Reject the second allocation and verify the first file is already trap-owned.
new_case restore-tempfail production
run_script restore-rehearsal.sh 51
[ -f "$CASE/first-temp" ] || fail 'first temporary restore file was not created'
grep -q '^mktemp-rejected$' "$CASE/events" || fail 'second allocation failure was not injected'
! grep -q '^restic ' "$CASE/events" || fail 'allocation failure reached Restic'
grep -q '^docker rm -f geoguessme-restore-' "$CASE/events" || fail 'allocation failure bypassed cleanup'

restore_case restore-gzip 47 0
restore_case restore-psql 49 0
restore_case restore-identity-gzip 48 1
restore_case restore-identity-psql 50 1
restore_case restore-success 0 2
restore_case restore-success 0 1 dev

# Exercise the actual deploy entrypoint: failed/partial Compose output must not
# become a successful "no database" result, nor advance to backup or deployment.
mkdir -p "$TMP/deploy-scripts"
cp "$ROOT/deployment/scripts/hosted/deploy.sh" "$TMP/deploy-scripts/deploy.sh"
cat >"$TMP/deploy-scripts/common.sh" <<'COMMON'
APP_ROOT="$CASE/app"
STATE_ROOT="$CASE/state"
CONFIG_ROOT="$CASE/config"
SECRET_ROOT="$CASE/secrets"
LOCK_ROOT="$CASE/locks"
COSIGN_IMAGE=fixture-cosign
SOPS_BOOTSTRAP_IMAGE=fixture-bootstrap
validate_environment() { :; }
validate_image_reference() { :; }
validate_sops_image_reference() { :; }
validate_dependency_image_reference() { :; }
valid_image_reference() { return 0; }
require_secret_file() { :; }
flock() { :; }
prune_releases() { :; }
release_dir() { printf '%s/releases/%s\n' "$APP_ROOT" "$1"; }
environment_env_file() { printf '%s/%s.env\n' "$SECRET_ROOT" "$1"; }
environment_port() { printf '8082\n'; }
oidc_enabled() { return 1; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
select_postgres_image() {
    printf 'select-postgres:%s\n' "${POSTGRES_IMAGE:-}" >>"$CASE/events"
    if [ "$MODE" = deploy-selection ]; then
        printf 'signature selection rejected\n' >&2
        return 76
    fi
    printf '%s\n' "${POSTGRES_IMAGE:-previous-postgres}"
}
select_restic_image() { printf 'previous-restic\n'; }
docker() {
    printf 'docker:%s\n' "$*" >>"$CASE/events"
    case "$1" in
        login) cat >/dev/null ;;
        run | pull) ;;
        *) die 'unexpected deploy Docker operation' ;;
    esac
}
curl() { printf 'health\n' >>"$CASE/events"; }
compose() {
    shift 2
    printf 'compose:%s:%s\n' "${POSTGRES_IMAGE:-}" "$*" >>"$CASE/events"
    if [ "$1" = ps ]; then
        [ "${POSTGRES_IMAGE:-}" = previous-postgres ] || die 'probe selected candidate database'
        case "$MODE" in
            deploy-compose) return 75 ;;
            deploy-partial) printf 'database-container-id\n'; return 75 ;;
            deploy-signature) die 'database signature rejected' ;;
            deploy-empty) return 0 ;;
        esac
        printf 'database-container-id\n'
    else
        [ "${POSTGRES_IMAGE:-}" = candidate-postgres ] || die 'deployment selected previous database'
    fi
}
COMMON
cat >"$TMP/deploy-scripts/backup.sh" <<'BACKUP'
#!/bin/sh
set -eu
. "$(dirname "$0")/common.sh"
selected=$(select_postgres_image "$1")
printf 'backup:%s:%s:%s\n' "$1" "$2" "$selected" >>"$CASE/events"
BACKUP
chmod +x "$TMP/deploy-scripts/backup.sh"

deploy_probe_case() {
    local mode=$1 expected=$2 actual=0
    export CASE MODE
    MODE=$mode CASE="$TMP/$mode"
    local revision=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    local metadata="$CASE/state/releases/dev/current.env"
    mkdir -p "$CASE/app/releases/$revision" "$CASE/state/releases/dev" \
        "$CASE/secrets" "$CASE/locks"
    touch "$CASE/events"
    printf 'GHCR_USERNAME=fixture\nGHCR_TOKEN=fixture\n' >"$CASE/secrets/dev.env"
    printf 'REVISION=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n' >"$metadata"
    cp "$metadata" "$CASE/original.env"
    if [ "$mode" != deploy-first ]; then
        mkdir -p "$CASE/app/dev" "$CASE/app/previous"
        ln -s "$CASE/app/previous" "$CASE/app/dev/current"
    fi
    "$TMP/deploy-scripts/deploy.sh" dev fixture-backend fixture-web fixture-sops \
        candidate-postgres candidate-restic "$revision" >"$CASE/output" 2>&1 || actual=$?
    if [ "$actual" -ne "$expected" ]; then
        cat "$CASE/output" >&2
        fail "$mode: expected deploy exit $expected, got $actual"
    fi
    if [ "$expected" -ne 0 ]; then
        ! grep -q '^backup:' "$CASE/events" || fail "$mode started backup after failed probe"
        ! grep -q '^compose:.*:\(pull\|run\|up\) ' "$CASE/events" ||
            fail "$mode started deployment after failed probe"
        ! grep -q '^health$' "$CASE/events" || fail "$mode reached health verification"
        ! grep -q 'first deployment:\|deployment completed:' "$CASE/output" ||
            fail "$mode misreported the failed probe as success"
        cmp -s "$metadata" "$CASE/original.env" || fail "$mode changed current metadata"
    else
        grep -q 'deployment completed:' "$CASE/output" || fail "$mode did not complete deployment"
        grep -q '^compose:candidate-postgres:run --rm migration migrate up$' "$CASE/events" ||
            fail "$mode did not migrate with candidate database selection"
        if [ "$mode" = deploy-existing ]; then
            grep -qx 'backup:dev:pre-deploy:previous-postgres' "$CASE/events" ||
                fail 'existing database backup did not select previous PostgreSQL'
            local backup_line migration_line
            backup_line=$(grep -n '^backup:' "$CASE/events" | cut -d: -f1)
            migration_line=$(grep -n '^compose:.*:run ' "$CASE/events" | cut -d: -f1)
            [ "$backup_line" -lt "$migration_line" ] || fail 'migration preceded backup'
        else
            ! grep -q '^backup:' "$CASE/events" || fail "$mode backed up an absent database"
            grep -q 'first deployment: no database exists' "$CASE/output" || fail "$mode omitted first-deploy result"
            if [ "$mode" = deploy-first ]; then
                ! grep -q ':ps ' "$CASE/events" || fail 'first deployment probed a missing current tree'
            fi
        fi
    fi
}

deploy_probe_case deploy-compose 1
deploy_probe_case deploy-partial 1
deploy_probe_case deploy-signature 1
deploy_probe_case deploy-selection 76
deploy_probe_case deploy-empty 0
deploy_probe_case deploy-first 0
deploy_probe_case deploy-existing 0

printf 'hosted backup, restore and deploy-probe pipeline contracts passed\n'
