#!/usr/bin/env bash
# Real Make recipes, fake selection/Docker/audit: no build, registry or live data.
# Called by Dockerized make test-security-workflows.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TMP=$(mktemp -d)
[[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] || exit 1
cleanup() { [[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] && rm -rf -- "$TMP"; }
trap cleanup EXIT
FIXTURE="$TMP/repository"
mkdir -p "$FIXTURE/tools/make" "$FIXTURE/tools/quality/dependency-images" "$FIXTURE/tools/quality/image-audit" "$FIXTURE/deployment/images" "$FIXTURE/deployment/env" "$FIXTURE/deployment/caddy" "$FIXTURE/deployment/scripts/watch" "$TMP/bin"
cp "$ROOT/tools/make/"{setup,dependency-images,deployment,quality}.mk "$FIXTURE/tools/make/"
cp "$ROOT/deployment/images/runtime.tsv" "$FIXTURE/deployment/images/"
cp "$ROOT/tools/quality/dependency-images/with-selected.sh" "$FIXTURE/tools/quality/dependency-images/"
cat >"$FIXTURE/Makefile" <<'MAKE'
include tools/make/setup.mk
include tools/make/dependency-images.mk
include tools/make/deployment.mk
include tools/make/quality.mk
dev-s3-guard: ; @true
consumer-integration: ; @$(TEST_ENV) GEOGUESSME_E2E_PROJECTS=desktop sh -c 'printf "%s\n%s\n" "POSTGRES_IMAGE=$$POSTGRES_IMAGE" "GEOGUESSME_E2E_PROJECTS=$$GEOGUESSME_E2E_PROJECTS"'
MAKE
: >"$FIXTURE/deployment/env/identity.env"
printf 'OIDC_ENABLED=false\n' >"$FIXTURE/deployment/env/production.env"
cat >"$FIXTURE/tools/quality/dependency-images/image-ref.sh" <<'SH'
#!/usr/bin/env bash
printf 'geoguessme/%s:dependency-%064d\n' "$2" 0
SH
cat >"$FIXTURE/tools/quality/dependency-images/lifecycle.sh" <<'SH'
#!/usr/bin/env bash
printf 'prepare|%s\n' "$*" >>"$TRACE"
SH
cat >"$FIXTURE/tools/quality/dependency-images/selected.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'select|%s\n' "$*" >>"$TRACE"
[[ "${FAIL_COMPONENT:-}" != "$1" ]] || { echo 'selection unavailable' >&2; exit 1; }
case "$1" in postgres) n=1 ;; keycloak) n=2 ;; caddy-runtime) n=3 ;; restic) n=4 ;; sops) n=5 ;; cloudflared) n=6 ;; socket-proxy) n=7 ;; *) exit 9 ;; esac
key=$(printf '%s_IMAGE' "$1" | tr '[:lower:]-' '[:upper:]_')
existing=${!key:-}
if [[ "$existing" == *@sha256:* ]]; then printf '%s\n' "$existing"
elif [[ "$1" == caddy-runtime && "${2:-}" == build ]]; then printf 'geoguessme/caddy-runtime:config-%064d\n' "$n"
else printf 'sha256:%064d\n' "$n"; fi
SH
cat >"$TMP/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker|%s|PG=%s|IDENTITY=%s|KEY=%s|GEOKEY=%s|CADDY=%s|SOPS=%s|CLOUD=%s|SOCKET=%s\n' "$*" "${POSTGRES_IMAGE:-}" "${IDENTITY_POSTGRES_IMAGE:-}" "${KEYCLOAK_IMAGE:-}" "${GEOGUESSME_KEYCLOAK_IMAGE:-}" "${CADDY_RUNTIME_IMAGE:-}" "${SOPS_IMAGE:-}" "${CLOUDFLARED_IMAGE:-}" "${SOCKET_PROXY_IMAGE:-}" >>"$TRACE"
SH
cat >"$FIXTURE/tools/quality/image-audit/audit.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "${IMAGE_AUDIT_REFS:?}" >"$AUDIT_CAPTURE"
printf 'audit\n' >>"$TRACE"
[[ "$IMAGE_AUDIT_REFS" != *'!missing-selection-'* ]] || exit 2
SH
cat >"$FIXTURE/deployment/caddy/init-local-tls.sh" <<'SH'
#!/usr/bin/env bash
exit 0
SH
for script in backup-restore-rehearsal restart-rehearsal reconnect-rehearsal migration-concurrency load-test container-verify prod-container-verify smoke-rehearsal; do
    cp "$TMP/bin/docker" "$FIXTURE/deployment/scripts/$script.sh"
done
cp "$TMP/bin/docker" "$FIXTURE/deployment/scripts/watch/rehearsal.sh"
mkdir -p "$FIXTURE/tools/quality/test"
cp "$FIXTURE/deployment/caddy/init-local-tls.sh" "$FIXTURE/tools/quality/test/check-tool-image-split.sh"
chmod +x "$FIXTURE/tools/quality/test/check-tool-image-split.sh"
chmod +x "$TMP/bin/docker" "$FIXTURE/deployment/caddy/init-local-tls.sh" "$FIXTURE/deployment/scripts/"*.sh "$FIXTURE/deployment/scripts/watch/rehearsal.sh"
export PATH="$TMP/bin:$PATH" TRACE="$TMP/trace" AUDIT_CAPTURE="$TMP/audit"
unset KEYCLOAK_IMAGE RESTIC_IMAGE SOPS_IMAGE SOCKET_PROXY_IMAGE POSTGRES_IMAGE CLOUDFLARED_IMAGE CADDY_RUNTIME_IMAGE IDENTITY_POSTGRES_IMAGE GEOGUESSME_GOOGLE_CLIENT_JSON AUDIT_APPLICATION_IMAGES
PASS=0
fail() {
    printf 'FAIL: consumer contract: %s\n' "$*" >&2
    exit 1
}
pass() {
    PASS=$((PASS + 1))
    printf 'PASS: %s\n' "$*"
}
contains() { grep -Fq -- "$2" "$1" || fail "missing $2"; }
absent() { ! grep -Fq -- "$2" "$1" || fail "unexpected $2"; }
run() {
    : >"$TRACE"
    make -s -C "$FIXTURE" "$@" >"$TMP/output" 2>"$TMP/error" || {
        sed -n '1,80p' "$TMP/error" >&2
        fail "Make $* failed"
    }
}
id() { printf 'sha256:%064d' "$1"; }
run build-images
contains "$TRACE" 'select|caddy-runtime build'
contains "$TRACE" "--build-arg CADDY_RUNTIME_IMAGE=geoguessme/caddy-runtime:config-$(printf '%064d' 3)"
contains "$TRACE" "docker|tag $(id 2) geoguessme-keycloak:local-"
absent "$TRACE" '--build-arg CADDY_RUNTIME_IMAGE=geoguessme/caddy-runtime:dependency-'
pass 'Make frontend FROM and checkout Keycloak tag consume selected bytes rather than input-key tags'
run_fail() {
    : >"$TRACE"
    if make -s -C "$FIXTURE" "$@" >"$TMP/output" 2>"$TMP/error"; then fail "Make $* accepted failed selection"; fi
}
FAIL_COMPONENT=caddy-runtime run_fail build-images
absent "$TRACE" 'docker|build '
pass 'failed frontend selection stops the command before either application build'
for target in dev dev-social identity-up; do
    run "$target"
    contains "$TRACE" "PG=$(id 1)"
    if [[ "$target" != dev ]]; then
        contains "$TRACE" "IDENTITY=$(id 1)"
        contains "$TRACE" "KEY=$(id 2)"
    fi
    if [[ "$target" == dev-social ]]; then contains "$TRACE" "CADDY=$(id 3)"; fi
    pass "$target starts only frozen prepared database and social runtime bytes"
done
FAIL_COMPONENT=postgres run_fail dev
absent "$TRACE" 'docker|compose '
pass 'failed database selection prevents Compose startup'
for target in down identity-down identity-config status logs; do
    run "$target"
    absent "$TRACE" 'select|'
    absent "$TRACE" 'prepare|'
done
pass 'down, logs, status and pure config remain available without prepared images'
run consumer-integration
contains "$TMP/output" "POSTGRES_IMAGE=$(id 1)"
contains "$TMP/output" 'GEOGUESSME_E2E_PROJECTS=desktop'
pass 'existing integration/E2E TEST_ENV prefix preserves arguments and exports frozen PostgreSQL'
run lint-caddy
contains "$TRACE" "CADDY=$(id 3)"
run tools-self-test
contains "$TRACE" "CADDY=$(id 3)"
contains "$TRACE" "CLOUD=$(id 6)"
contains "$TRACE" "SOPS=$(id 5)"
pass 'Caddy lint and utility self-tests consume immutable selections after explicit preparation'
for target in backup-rehearsal restart-rehearsal reconnect-rehearsal migration-test container-verify prod-container-verify smoke-rehearsal load-test; do
    run "$target"
    contains "$TRACE" "PG=$(id 1)"
done
run watch-rehearsal
contains "$TRACE" "PG=$(id 1)"
contains "$TRACE" "SOCKET=$(id 7)"
pass 'disposable operational consumers receive frozen database and socket-proxy references'
run audit-images
for n in {1..7}; do contains "$AUDIT_CAPTURE" "$(id "$n")"; done
contains "$AUDIT_CAPTURE" 'geoguessme-backend:local-'
contains "$AUDIT_CAPTURE" 'geoguessme-web:local-'
absent "$TRACE" 'prepare|'
absent "$TRACE" 'docker|'
[[ "$(awk '{print NF}' "$AUDIT_CAPTURE")" == 17 ]] || fail 'default audit omitted required refs'
pass 'default audit receives all seventeen immutable/runtime/app refs without preparing anything'
FAIL_COMPONENT=restic run_fail audit-images
contains "$AUDIT_CAPTURE" '!missing-selection-restic'
contains "$AUDIT_CAPTURE" "$(id 7)"
contains "$TRACE" 'audit'
[[ "$(grep -c '^select|' "$TRACE")" == 7 ]] || fail 'missing selection truncated remaining components'
[[ "$(awk '{print NF}' "$AUDIT_CAPTURE")" == 17 ]] || fail 'incomplete audit reduced scope'
absent "$TRACE" 'prepare|'
pass 'missing local selection records an invalid required ref while all remaining images reach the audit'
remote="ghcr.io/anko59/geoguessme-caddy-runtime:dependency-$(printf '%064d' 0)@sha256:$(printf '%064d' 9)"
CADDY_RUNTIME_IMAGE="$remote" run build-images
contains "$TRACE" "--build-arg CADDY_RUNTIME_IMAGE=$remote"
CADDY_RUNTIME_IMAGE="$remote" run audit-images
contains "$AUDIT_CAPTURE" "$remote"
pass 'explicit trusted CI digest is preserved through both frontend FROM and default audit'
run audit-images AUDIT_APPLICATION_IMAGES=false
[[ "$(awk '{print NF}' "$AUDIT_CAPTURE")" == 15 ]] || fail 'explicit standalone dependency audit changed scope'
pass 'explicit standalone dependency-only audit remains fifteen refs'
for invalid in '' tru TRUE 'true false' 'false true' 'true '; do
    run_fail audit-images "AUDIT_APPLICATION_IMAGES=$invalid"
    contains "$TMP/error" 'POLICY: AUDIT_APPLICATION_IMAGES must be true or false'
    absent "$TRACE" 'audit'
    absent "$TRACE" 'select|'
    absent "$TRACE" 'prepare|'
    absent "$TRACE" 'docker|'
done
pass 'invalid application-audit flags fail before selection or any partial-scope audit'
printf 'selected consumer contracts PASSED (%s checks)\n' "$PASS"
