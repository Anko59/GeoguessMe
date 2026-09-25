#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "$0")/../../.." && pwd)
script="$root_dir/tools/ops/credentials.sh"
test_dir=$(mktemp -d)
trap 'rm -rf -- "$test_dir"' EXIT INT TERM
fake_bin="$test_dir/bin"
mkdir -p "$fake_bin"

cat >"$fake_bin/secret-tool" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == 'lookup service codex-api provider cloudflare project geoguessme name CLOUDFLARE_API_TOKEN' ]] || exit 2
[[ -n "${TEST_KEYRING_TOKEN:-}" ]] || exit 1
printf '%s\n' "$TEST_KEYRING_TOKEN"
STUB

cat >"$fake_bin/ssh-add" <<'STUB'
#!/usr/bin/env bash
[[ "${TEST_SSH_AGENT:-}" == available ]]
STUB

cat >"$fake_bin/docker" <<'STUB'
#!/usr/bin/env bash
[[ "${TEST_DOCKER_COMPOSE:-}" == available && "$*" == 'compose version' ]]
STUB

cat >"$fake_bin/gh" <<'STUB'
#!/usr/bin/env bash
[[ "${TEST_GH_AUTH:-}" == available && "$*" == 'auth status --hostname github.com' ]]
STUB

for command_name in curl jq ssh make; do
    printf '#!/usr/bin/env bash\nexit 0\n' >"$fake_bin/$command_name"
done
chmod 700 "$fake_bin"/*

base_environment=(PATH="$fake_bin:/usr/bin:/bin" TEST_SSH_AGENT=available TEST_DOCKER_COMPOSE=available TEST_GH_AUTH=available)
keyring_output=$(env -u CLOUDFLARE_API_TOKEN "${base_environment[@]}" \
    TEST_KEYRING_TOKEN=credential-fixture-not-for-output bash "$script" preflight)
[[ "$keyring_output" == *'Cloudflare API token: available via GNOME Secret Service keyring'* ]] || {
    echo 'preflight did not resolve the documented keyring item' >&2
    exit 1
}
[[ "$keyring_output" != *'credential-fixture-not-for-output'* ]] || {
    echo 'preflight printed a credential value' >&2
    exit 1
}
[[ "$keyring_output" == *'Operator SSH identity: available from ssh-agent'* ]] || {
    echo 'preflight did not report the loaded operator identity' >&2
    exit 1
}
[[ "$keyring_output" == *'GitHub CLI authentication: available'* ]] || {
    echo 'preflight did not report existing GitHub CLI authentication' >&2
    exit 1
}

environment_output=$(env "${base_environment[@]}" TEST_KEYRING_TOKEN=unused-fixture \
    CLOUDFLARE_API_TOKEN=environment-fixture-not-for-output bash "$script" preflight)
[[ "$environment_output" == *'Cloudflare API token: available via environment'* ]] || {
    echo 'preflight did not prefer an explicitly supplied API token' >&2
    exit 1
}
[[ "$environment_output" != *'environment-fixture-not-for-output'* ]] || {
    echo 'preflight printed an environment credential value' >&2
    exit 1
}

if missing_output=$(env -u CLOUDFLARE_API_TOKEN "${base_environment[@]}" \
    TEST_KEYRING_TOKEN= TEST_SSH_AGENT=missing TEST_DOCKER_COMPOSE=missing \
    bash "$script" preflight 2>&1); then
    echo 'preflight accepted missing operator credentials and tooling' >&2
    exit 1
fi
[[ "$missing_output" == *'Cloudflare API token: unavailable'* ]] || {
    echo 'preflight did not identify the unavailable API token source' >&2
    exit 1
}
[[ "$missing_output" == *'Operator SSH identity: unavailable'* ]] || {
    echo 'preflight did not identify the unavailable SSH identity' >&2
    exit 1
}
[[ "$missing_output" == *'Docker Compose: unavailable'* ]] || {
    echo 'preflight did not identify unavailable Docker Compose' >&2
    exit 1
}

grep -Fq 'duration: "1h"' "$script"
grep -Fq 'decision: "non_identity"' "$script"
grep -Fq "wait_for_access_policy \"\$hostname\"" "$script"
grep -Fq 'trap cleanup_access_session EXIT' "$script"
grep -Fq 'unset TUNNEL_SERVICE_TOKEN_ID TUNNEL_SERVICE_TOKEN_SECRET' "$script"
grep -Fq "2>'\$scratch_dir/cloudflared.stderr'" "$script"

echo 'operator credential preflight checks passed'
