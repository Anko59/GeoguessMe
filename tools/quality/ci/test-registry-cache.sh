#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../.." && pwd)
helper="$root/tools/quality/ci/registry-cache.sh"
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
printf '{"features":{"containerd-snapshotter":true},"registry-mirrors":["https://existing.example"]}\n' >"$tmp/input.json"
bash "$helper" --render "$tmp/input.json" >"$tmp/first.json"
jq -e '.features["containerd-snapshotter"] == true and .["registry-mirrors"] == ["https://mirror.gcr.io","https://existing.example"]' "$tmp/first.json" >/dev/null
bash "$helper" --render "$tmp/first.json" >"$tmp/second.json"
cmp "$tmp/first.json" "$tmp/second.json"
for invalid in '[]' '{"registry-mirrors":{}}' '{"registry-mirrors":[42]}' 'invalid'; do
    printf '%s\n' "$invalid" >"$tmp/invalid.json"
    if bash "$helper" --render "$tmp/invalid.json" >/dev/null 2>&1; then
        echo 'Invalid Docker configuration was accepted' >&2
        exit 1
    fi
done
if GITHUB_ACTIONS=false RUNNER_ENVIRONMENT=github-hosted RUNNER_OS=Linux bash "$helper" >/dev/null 2>&1; then
    echo 'Registry cache mutated a non-CI host' >&2
    exit 1
fi
mkdir "$tmp/bin"
cat >"$tmp/bin/docker" <<'STUB'
#!/bin/sh
case "$TEST_DOCKER_STATE" in
    error) exit 1 ;;
    live) printf 'running-container\n' ;;
esac
STUB
cat >"$tmp/bin/sudo" <<'STUB'
#!/bin/sh
touch "$TEST_MUTATION_MARKER"
exit 1
STUB
chmod +x "$tmp/bin/docker" "$tmp/bin/sudo"
for state in error live; do
    if PATH="$tmp/bin:$PATH" TEST_DOCKER_STATE="$state" TEST_MUTATION_MARKER="$tmp/mutated" GITHUB_ACTIONS=true RUNNER_ENVIRONMENT=github-hosted RUNNER_OS=Linux bash "$helper" >/dev/null 2>&1; then
        echo "Unsafe Docker state was accepted: $state" >&2
        exit 1
    fi
    test ! -e "$tmp/mutated"
done
action="$root/.github/actions/setup-buildx/action.yml"
grep -Fq 'run: make ci-registry-cache' "$action"
grep -Fq 'mirrors = ["mirror.gcr.io"]' "$action"
grep -Fq 'uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069' "$action"
if grep -Rq 'uses: docker/setup-buildx-action@' "$root/.github/workflows"; then
    echo 'A workflow bypasses the shared Docker pull-cache setup' >&2
    exit 1
fi
echo 'CI registry-cache contracts PASSED: preservation, idempotence, invalid configuration, host guard, daemon and Buildx paths'
