#!/usr/bin/env bash
# Exercise the actual wrapper with a private dummy repository and no daemon.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TMP=$(mktemp -d /tmp/geoguessme-cloud-init-runner.XXXXXXXX)
cleanup() {
    case "$TMP" in /tmp/geoguessme-cloud-init-runner.*)
        [[ -d "$TMP" && ! -L "$TMP" && "$(realpath "$TMP")" == "$TMP" ]] || return
        rm -rf -- "$TMP"
        ;;
    esac
}
trap cleanup EXIT
repo="$TMP/repo"
mkdir -p "$repo/tools/quality/cloud-init" "$repo/infra/terraform" "$TMP/bin"
for file in tools/quality/cloud-init/run-test.sh tools/quality/cloud-init/Dockerfile \
    tools/quality/cloud-init/test-user-data.py infra/terraform/versions.tf \
    infra/terraform/.terraform.lock.hcl; do
    cp "$ROOT/$file" "$repo/$file"
done
git -C "$repo" init -q
git -C "$repo" add -- tools/quality/cloud-init/Dockerfile infra/terraform/versions.tf infra/terraform/.terraform.lock.hcl
# These are synthetic untracked sentinels, not real operator secrets/state.
printf 'untracked-operator-sentinel\n' >"$repo/infra/terraform/private.auto.tfvars"
printf 'untracked-state-sentinel\n' >"$repo/infra/terraform/terraform.tfstate"
printf 'untracked-dotenv-sentinel\n' >"$repo/.env"
cat >"$TMP/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$1" >>"$CALLS"
case "$1" in
    build)
        shift
        iid=''
        while (($#)); do
            if [[ "$1" == --iidfile ]]; then iid=$2; shift; fi
            shift
        done
        [[ -n "$iid" ]] || exit 61
        # Docker's IID file need not end with a newline. The mutable tag would
        # point at different bytes; the wrapper must never inspect that tag.
        printf '%s' "sha256:$(printf 'a%.0s' {1..64})" >"$iid"
        printf '%s\n' "${iid%/*}" >"$ALLOCATION"
        ;;
    run)
        [[ "${*: -1}" == "sha256:$(printf 'a%.0s' {1..64})" ]] || exit 62
        [[ " $* " == *' --network none '* ]] || exit 63
        workspace="$(<"$ALLOCATION")/workspace"
        for file in infra/terraform/private.auto.tfvars infra/terraform/terraform.tfstate .env .git; do
            [[ ! -e "$workspace/$file" ]] || exit 64
        done
        [[ -f "$workspace/tools/quality/cloud-init/test-user-data.py" ]] || exit 65
        if [[ "$SIGNAL" != none ]]; then kill -s "$SIGNAL" "$PPID"; fi
        # A successful child must not convert parent cancellation into success.
        exit 0
        ;;
    *) exit 66 ;;
esac
MOCK
chmod +x "$TMP/bin/docker"
export PATH="$TMP/bin:$PATH"
export CALLS="$TMP/calls" ALLOCATION="$TMP/allocation"
for SIGNAL in none TERM INT; do
    export SIGNAL
    : >"$CALLS"
    status=0
    bash "$repo/tools/quality/cloud-init/run-test.sh" >"$TMP/output" 2>&1 || status=$?
    case "$SIGNAL" in none) expected=0 ;; TERM) expected=143 ;; INT) expected=130 ;; esac
    [[ "$status" == "$expected" ]] || {
        printf 'cloud-init runner %s: expected %s, got %s\n' "$SIGNAL" "$expected" "$status" >&2
        exit 1
    }
    [[ "$(<"$CALLS")" == $'build\nrun' ]] || exit 1
    [[ ! -e "$(<"$ALLOCATION")" ]] || exit 1
done
printf 'Cloud-init runner immutable IID, private sources, cleanup and cancellation contracts PASS\n'
