#!/usr/bin/env bash
# Host Docker orchestration only; the S3 client runs in the pinned Go container.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$ROOT"
PROJECT="geoguessme-s3-fixture-regression-$$"
case "$PROJECT" in geoguessme-s3-fixture-regression-[0-9]*) ;; *) exit 2 ;; esac
compose() { docker compose -p "$PROJECT" -f deployment/compose.test.yaml --project-directory "$ROOT" "$@"; }
client() {
    make --no-print-directory -s s3-fixture \
        GEOGUESSME_S3_FIXTURE_NETWORK="${PROJECT}_default" \
        S3_FIXTURE_ENDPOINT=http://minio:9000 \
        SOURCE_S3_FIXTURE_ENDPOINT=http://minio:9000 \
        SOURCE_S3_FIXTURE_ACCESS_KEY=minioadmin \
        SOURCE_S3_FIXTURE_SECRET_KEY=minioadmin \
        S3_FIXTURE_COMMAND="$*"
}
cleanup() {
    compose logs --no-color minio >"$ROOT/.local/s3-fixture-last.log" 2>&1 || true
    # This exact uniquely allocated disposable project never contains dev data.
    compose down --volumes --remove-orphans
    case "${LITERAL_TMP:-}" in "$ROOT/.local/s3-literal-args."*) rm -rf -- "$LITERAL_TMP" ;; *) return 1 ;; esac
}
mkdir -p "$ROOT/.local"
LITERAL_TMP=$(mktemp -d "$ROOT/.local/s3-literal-args.XXXXXXXX")
trap cleanup EXIT
compose up -d --wait --wait-timeout 120 minio
client ensure fixture-source
printf '%s' 'fixture-integrity-payload' | client put fixture-source media/test.dat -
[[ "$(client get fixture-source media/test.dat)" == fixture-integrity-payload ]]
if printf '%s' 'unapproved overwrite' | client put fixture-source media/test.dat -; then
    echo 'fixture client overwrote an existing object' >&2
    exit 1
fi
[[ "$(client get fixture-source media/test.dat)" == fixture-integrity-payload ]]
if client get fixture-source media/test.dat S3_FIXTURE_ACCESS_KEY=wrong; then
    # Credential rejection is exercised below with a real environment override.
    echo 'fixture client accepted excess arguments' >&2
    exit 1
fi
if make --no-print-directory -s s3-fixture \
    GEOGUESSME_S3_FIXTURE_NETWORK="${PROJECT}_default" \
    S3_FIXTURE_ACCESS_KEY=invalid S3_FIXTURE_SECRET_KEY=invalid \
    S3_FIXTURE_COMMAND='get fixture-source media/test.dat'; then
    echo 'fixture accepted invalid credentials' >&2
    exit 1
fi
if compose exec -T minio curl --fail --silent --show-error --max-time 2 \
    http://127.0.0.1:9000/fixture-source/media/test.dat; then
    echo 'private fixture object was anonymously readable' >&2
    exit 1
fi
client copy fixture-source fixture-target
client verify fixture-source fixture-target
client copy fixture-source fixture-target
# Literal data must survive both Make and Docker, without shell/Make evaluation.
printf '%s' 'literal-json-key-payload' | make --no-print-directory -s s3-fixture \
    GEOGUESSME_S3_FIXTURE_NETWORK="${PROJECT}_default" \
    S3_FIXTURE_ARGS_JSON="[\"put\",\"fixture-source\",\"media/key with spaces quotes\\\" \$(shell never-evaluate) \$HOME\", \"-\"]"
json_payload=$(make --no-print-directory -s s3-fixture \
    GEOGUESSME_S3_FIXTURE_NETWORK="${PROJECT}_default" \
    S3_FIXTURE_ARGS_JSON="[\"get\",\"fixture-source\",\"media/key with spaces quotes\\\" \$(shell never-evaluate) \$HOME\"]")
[[ "$json_payload" == literal-json-key-payload ]]
literal_args="[\"get\",\"fixture-source\",\"media/\$(shell touch $LITERAL_TMP/canary)\"]"
if make --no-print-directory -s s3-fixture \
    GEOGUESSME_S3_FIXTURE_NETWORK="${PROJECT}_default" S3_FIXTURE_ARGS_JSON="$literal_args"; then
    echo 'unexpected literal canary object exists' >&2
    exit 1
fi
[[ ! -e "$LITERAL_TMP/canary" ]] || {
    echo 'Make evaluated an object key as code' >&2
    exit 1
}
client copy fixture-source fixture-target
compose restart minio
compose up -d --wait --wait-timeout 120 minio
[[ "$(client get fixture-target media/test.dat)" == fixture-integrity-payload ]]
client verify fixture-source fixture-target
printf '%s' 'unexpected extra object' | client put fixture-target media/extra.dat -
if client verify fixture-source fixture-target; then
    echo 'migration verification ignored an extra target object' >&2
    exit 1
fi
echo 'S3 fixture authentication, integrity, no-overwrite, migration and restart PASS'
