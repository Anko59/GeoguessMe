#!/bin/sh
# Deterministic state contracts for adoption, backup selection and rollback.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
TMP=$(mktemp -d /tmp/geoguessme-dependencies.XXXXXX)
cleanup() {
    case "$TMP" in /tmp/geoguessme-dependencies.*) rm -rf -- "$TMP" ;; *) exit 1 ;; esac
}
trap cleanup EXIT INT TERM
fail() {
    printf 'dependency image contract failed: %s\n' "$*" >&2
    exit 1
}
mkdir -p "$TMP/bin" "$TMP/home/.docker" "$TMP/app/bin" "$TMP/app/config" \
    "$TMP/state/releases/dev" "$TMP/state/releases/production" "$TMP/secrets" "$TMP/locks"
cp "$ROOT/deployment/scripts/hosted/common.sh" "$TMP/app/bin/common.sh"
cp "$ROOT/deployment/scripts/hosted/deploy.sh" "$TMP/app/bin/deploy.sh"
old=$(printf 'a%.0s' $(seq 1 40))
new=$(printf 'b%.0s' $(seq 1 40))
digest=$(printf 'c%.0s' $(seq 1 64))
old_pg="ghcr.io/anko59/geoguessme-postgres:dev-$old@sha256:$digest"
new_pg="ghcr.io/anko59/geoguessme-postgres:dev-$new@sha256:$digest"
old_restic="ghcr.io/anko59/geoguessme-restic:dev-$old@sha256:$digest"
new_restic="ghcr.io/anko59/geoguessme-restic:dev-$new@sha256:$digest"
prod_pg="ghcr.io/anko59/geoguessme-postgres:release-$old@sha256:$digest"
prod_restic="ghcr.io/anko59/geoguessme-restic:release-$old@sha256:$digest"
sops="ghcr.io/anko59/geoguessme-sops:dev-$new@sha256:$digest"
app="ghcr.io/anko59/geoguessme-backend:dev-$new@sha256:$digest"
web="ghcr.io/anko59/geoguessme-web:dev-$new@sha256:$digest"
mkdir -p "$TMP/app/releases/$old" "$TMP/app/releases/$new/deployment/oauth2-proxy" "$TMP/app/dev"
for name in oauth2-proxy.cfg oauth2-proxy-alpha.yaml; do
    printf 'public fixture\n' >"$TMP/app/releases/$new/deployment/oauth2-proxy/$name"
    chmod 600 "$TMP/app/releases/$new/deployment/oauth2-proxy/$name"
done
printf '%s\n' "$old" >"$TMP/app/config/runtime-revision"
ln -s "$TMP/app/releases/$old" "$TMP/app/dev/current"
printf 'GHCR_USERNAME=fixture\nGHCR_TOKEN=fixture\n' >"$TMP/secrets/dev.env"
chmod 600 "$TMP/secrets/dev.env"
printf 'fixture\n' >"$TMP/secrets/identity.env"
cat >"$TMP/bin/docker" <<'DOCKER'
#!/bin/sh
set -eu
printf 'docker:%s\n' "$*" >>"$TRACE"
case "$1" in
    run)
        case " $* " in
            *' verify '*)
                for arg do last=$arg; done
                [ "$last" != "${FAIL_SIGNATURE:-}" ] || exit 1
                ;;
            *' /usr/bin/restic '*) printf 'restic:%s\n' "$*" >>"$TRACE" ;;
            *' decrypt '*) printf 'GEOGUESSME_DEV_OIDC_CLIENT_SECRET=fixture\nGEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET=fixture\n' ;;
            *) exit 1 ;;
        esac
        ;;
    login) while IFS= read -r line; do :; done ;;
    pull) printf 'pull:%s\n' "$2" >>"$TRACE" ;;
    ps) [ -z "${ACTIVE_DB_IMAGE:-}" ] || printf 'active-db\n' ;;
    inspect) printf '%s\n' "$ACTIVE_DB_IMAGE" ;;
    compose)
        printf 'compose-postgres:%s:%s\n' "${POSTGRES_IMAGE:-}" "$*" >>"$TRACE"
        case " $* " in
            *' ps --status running postgres --quiet '*) printf 'running-db\n' ;;
            *' up -d --wait '* )
                if [ "${FAIL_APPLY:-0}" = 1 ] && [ "${POSTGRES_IMAGE:-}" = "$NEW_PG" ]; then exit 1; fi
                ;;
        esac
        ;;
    *) exit 1 ;;
esac
DOCKER
cat >"$TMP/bin/curl" <<'CURL'
#!/bin/sh
exit 0
CURL
cat >"$TMP/bin/flock" <<'FLOCK'
#!/bin/sh
exit 0
FLOCK
cat >"$TMP/app/bin/backup.sh" <<'BACKUP'
#!/bin/sh
set -eu
. "$(dirname -- "$0")/common.sh"
printf 'backup-restic:%s\n' "$(select_restic_image "$1")" >>"$TRACE"
BACKUP
chmod 755 "$TMP/bin/docker" "$TMP/bin/curl" "$TMP/bin/flock" "$TMP/app/bin/backup.sh"
export PATH="$TMP/bin:$PATH" HOME="$TMP/home" TRACE="$TMP/trace" NEW_PG="$new_pg"
export GEOGUESSME_APP_ROOT="$TMP/app" GEOGUESSME_STATE_ROOT="$TMP/state"
export GEOGUESSME_SECRET_ROOT="$TMP/secrets" GEOGUESSME_LOCK_ROOT="$TMP/locks"
metadata="$TMP/state/releases/dev/current.env"
reset_metadata() {
    printf 'BACKEND_IMAGE=%s\nWEB_IMAGE=%s\nREVISION=%s\nPOSTGRES_IMAGE=%s\nRESTIC_IMAGE=%s\n' \
        "$app" "$web" "$old" "$old_pg" "$old_restic" >"$metadata"
    chmod 600 "$metadata"
    : >"$TRACE"
}
run_deploy() { sh "$TMP/app/bin/deploy.sh" dev "$app" "$web" "$sops" "$1" "$new_restic" "$new"; }
helper() { sh -c '. "$GEOGUESSME_APP_ROOT/bin/common.sh"; "$@"' sh "$@"; }
reset_metadata
if run_deploy "ghcr.io/attacker/postgres:dev-$new@sha256:$digest" >"$TMP/result" 2>&1; then
    fail 'foreign database repository was admitted'
fi
[ ! -s "$TRACE" ] || fail 'invalid reference reached Docker'
if run_deploy "$old_pg" >"$TMP/result" 2>&1; then fail 'wrong revision was admitted'; fi
[ ! -s "$TRACE" ] || fail 'wrong revision reached Docker'
if FAIL_SIGNATURE="$new_pg" run_deploy "$new_pg" >"$TMP/result" 2>&1; then
    fail 'database signature failure was ignored'
fi
! grep -Fq "pull:$new_pg" "$TRACE" || fail 'unverified database was pulled'
! grep -Fq "pull:$new_restic" "$TRACE" || fail 'Restic pulled before all dependency signatures passed'
grep -Fq "POSTGRES_IMAGE=$old_pg" "$metadata" || fail 'rejected candidate changed active state'
reset_metadata
if FAIL_SIGNATURE="$new_restic" run_deploy "$new_pg" >"$TMP/result" 2>&1; then
    fail 'Restic signature failure was ignored'
fi
! grep -Fq "pull:$new_pg" "$TRACE" || fail 'database pulled before Restic signature passed'
reset_metadata
run_deploy "$new_pg" >"$TMP/result" 2>&1 || {
    sed -n '1,80p' "$TMP/result" >&2
    fail 'valid adoption failed'
}
for name in oauth2-proxy.cfg oauth2-proxy-alpha.yaml; do
    public="$TMP/app/releases/$new/deployment/oauth2-proxy/$name"
    [ "$(stat -c '%a' "$public")" = 644 ] || fail 'public config permissions were not normalized'
    [ "$(cat "$public")" = 'public fixture' ] || fail 'public config bytes changed'
done
grep -Fxq "POSTGRES_IMAGE=$new_pg" "$metadata" || fail 'database candidate not persisted'
grep -Fxq "RESTIC_IMAGE=$new_restic" "$metadata" || fail 'Restic candidate not persisted'
grep -Fxq "SOPS_IMAGE=$sops" "$metadata" || fail 'SOPS candidate not persisted'
[ "$(stat -c '%a' "$metadata")" = 600 ] || fail 'active metadata permissions changed'
grep -Fxq "backup-restic:$old_restic" "$TRACE" || fail 'predeploy backup used candidate Restic'
verify_line=$(grep -nF "revision=$new $new_pg" "$TRACE" | head -1 | cut -d: -f1)
pull_line=$(grep -nFx "pull:$new_pg" "$TRACE" | cut -d: -f1)
[ "$verify_line" -lt "$pull_line" ] || fail 'database pull preceded signature verification'
reset_metadata
if FAIL_APPLY=1 run_deploy "$new_pg" >"$TMP/result" 2>&1; then fail 'application failure did not fail deploy'; fi
grep -Fxq "POSTGRES_IMAGE=$old_pg" "$metadata" || fail 'rollback retained candidate database metadata'
grep -Fxq "RESTIC_IMAGE=$old_restic" "$metadata" || fail 'rollback retained candidate Restic metadata'
grep -F "compose-postgres:$old_pg:" "$TRACE" | grep -q 'up -d --wait' || fail 'rollback did not select exact old database'
[ "$(RESTIC_IMAGE="$new_restic" helper select_restic_image dev)" = "$old_restic" ] || fail 'candidate environment overrode active backup image'
printf 'POSTGRES_IMAGE=%s\nRESTIC_IMAGE=%s\nREVISION=%s\n' "$prod_pg" "$prod_restic" "$old" >"$TMP/state/releases/production/current.env"
[ "$(POSTGRES_IMAGE="$new_pg" helper select_identity_postgres_image)" = "$prod_pg" ] || fail 'dev candidate leaked into identity DB'
[ "$(helper select_restic_image production)" = "$prod_restic" ] || fail 'production backup used dev image'
if FAIL_SIGNATURE="$old_restic" helper restic dev snapshots >"$TMP/result" 2>&1; then fail 'stored backup image skipped signature verification'; fi
if FAIL_SIGNATURE="$old_pg" helper select_postgres_image dev >"$TMP/result" 2>&1; then fail 'stored database image skipped signature verification'; fi
if FAIL_SIGNATURE="$prod_pg" helper select_identity_postgres_image >"$TMP/result" 2>&1; then fail 'identity database image skipped signature verification'; fi
printf 'RESTIC_IMAGE=%s\nRESTIC_IMAGE=%s\n' "$old_restic" "$new_restic" >"$metadata"
if helper select_restic_image dev >"$TMP/result" 2>&1; then fail 'duplicate active metadata was accepted'; fi
# Legacy rollback captures an actual active database, not the incoming digest.
printf 'BACKEND_IMAGE=%s\nWEB_IMAGE=%s\nREVISION=%s\n' "$app" "$web" "$old" >"$metadata"
legacy_pg='postgres:15-alpine@sha256:3d0f7584ed7d04e27fa050d6683a74746608faf21f202be78460d679cc56461f'
if ACTIVE_DB_IMAGE="$legacy_pg" FAIL_APPLY=1 run_deploy "$new_pg" >"$TMP/result" 2>&1; then fail 'legacy rollback fixture unexpectedly succeeded'; fi
grep -Fxq "POSTGRES_IMAGE=$legacy_pg" "$metadata" || {
    sed -n '1,100p' "$TMP/result" >&2
    fail 'legacy rollback guessed database image'
}
# Production and dev carry distinct full protocol arities in both entry points.
grep -Fq 'dev:7)' "$ROOT/deployment/scripts/hosted/forced-command.sh" || fail 'forced dev protocol missing'
grep -Fq 'production:8)' "$ROOT/deployment/scripts/hosted/forced-command.sh" || fail 'forced production protocol missing'
# Source contracts intentionally match unexpanded shell expressions.
# shellcheck disable=SC2016
grep -Fq 'restore_postgres=$(select_postgres_image "$environment")' "$ROOT/deployment/scripts/hosted/restore-rehearsal.sh" || fail 'restore uses a different app database'
# shellcheck disable=SC2016
grep -Fq 'restore_identity_postgres=$(select_identity_postgres_image)' "$ROOT/deployment/scripts/hosted/restore-rehearsal.sh" || fail 'restore uses a different identity database'
# Exercise shared-identity adoption and rollback through the real deploy script.
new_prod_pg="ghcr.io/anko59/geoguessme-postgres:release-$new@sha256:$digest"
new_prod_restic="ghcr.io/anko59/geoguessme-restic:release-$new@sha256:$digest"
old_keycloak="ghcr.io/anko59/geoguessme-keycloak:release-$old@sha256:$digest"
new_keycloak="ghcr.io/anko59/geoguessme-keycloak:release-$new@sha256:$digest"
prod_sops="ghcr.io/anko59/geoguessme-sops:release-$new@sha256:$digest"
mkdir -p "$TMP/app/production" "$TMP/app/releases/$new/deployment/secrets"
ln -s "$TMP/app/releases/$old" "$TMP/app/production/current"
: >"$TMP/app/releases/$new/deployment/secrets/identity.env.enc"
printf 'GEOGUESSME_DEV_OIDC_CLIENT_SECRET=fixture\nGEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET=fixture\n' >"$TMP/secrets/identity.env"
printf 'GHCR_USERNAME=fixture\nGHCR_TOKEN=fixture\nOIDC_ENABLED=true\nOIDC_CLIENT_SECRET=fixture\n' >"$TMP/secrets/production.env"
chmod 600 "$TMP/secrets/production.env" "$TMP/secrets/identity.env"
production_metadata="$TMP/state/releases/production/current.env"
reset_production() {
    printf 'BACKEND_IMAGE=%s\nWEB_IMAGE=%s\nREVISION=%s\nPOSTGRES_IMAGE=%s\nRESTIC_IMAGE=%s\nKEYCLOAK_IMAGE=%s\nIDENTITY_POSTGRES_IMAGE=%s\n' \
        "$app" "$web" "$old" "$prod_pg" "$prod_restic" "$old_keycloak" "$prod_pg" >"$production_metadata"
    : >"$TRACE"
}
run_production() {
    sh "$TMP/app/bin/deploy.sh" production "$app" "$web" "$new_keycloak" \
        "$prod_sops" "$new_prod_pg" "$new_prod_restic" "$new"
}
# Restore valid dev metadata for the release-pruning safety checks.
reset_metadata
reset_production
run_production >"$TMP/result" 2>&1 || {
    sed -n '1,100p' "$TMP/result" >&2
    fail 'production adoption failed'
}
grep -Fxq "IDENTITY_POSTGRES_IMAGE=$new_prod_pg" "$production_metadata" || fail 'shared identity candidate not persisted'
grep -F "compose-postgres:$new_prod_pg:" "$TRACE" | grep -q 'up -d --wait keycloak-db keycloak' || fail 'identity database used different candidate'
grep -Fxq "backup-restic:$prod_restic" "$TRACE" || fail 'production predeploy used a different backup artifact'
reset_production
if NEW_PG="$new_prod_pg" FAIL_APPLY=1 run_production >"$TMP/result" 2>&1; then fail 'production failure did not roll back'; fi
grep -Fxq "IDENTITY_POSTGRES_IMAGE=$prod_pg" "$production_metadata" || fail 'identity metadata retained failed candidate'
grep -F "compose-postgres:$prod_pg:" "$TRACE" | grep -q 'up -d --wait keycloak-db keycloak' || fail 'identity database rollback did not use old image'
printf 'Hosted dependency adoption, signature, backup and rollback contracts passed\n'
