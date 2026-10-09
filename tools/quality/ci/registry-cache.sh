#!/usr/bin/env bash
set -euo pipefail
umask 077

# CI uses the daemon for frozen local FROM aliases as well as isolated Buildx.
# Public mirrors serve identical digest-addressed bytes; cache misses retain
# Docker's upstream fallback and never relax the committed image pins.
render() {
    jq -e '
        if type != "object" then error("Docker configuration must be an object")
        elif (."registry-mirrors" // [] | type) != "array" then error("registry-mirrors must be an array")
        elif any((."registry-mirrors" // [])[]; type != "string") then error("registry mirror must be a string")
        else ."registry-mirrors" = (["https://mirror.gcr.io"] + ((."registry-mirrors" // []) | map(select(. != "https://mirror.gcr.io"))))
        end
    ' "$1"
}

if [[ "${1:-}" == --render && "$#" == 2 ]]; then
    render "$2"
    exit
fi
[[ "$#" == 0 ]] || {
    echo 'Unsupported registry-cache arguments' >&2
    exit 2
}
if [[ "${GITHUB_ACTIONS:-}" != true || "${RUNNER_ENVIRONMENT:-}" != github-hosted || "${RUNNER_OS:-}" != Linux ]]; then
    echo 'Registry-cache setup is restricted to ephemeral GitHub-hosted Linux runners' >&2
    exit 2
fi
containers=$(docker ps -q)
if [[ -n "$containers" ]]; then
    echo 'Configure the registry cache before starting any job containers' >&2
    exit 2
fi
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
if sudo test -f /etc/docker/daemon.json; then
    sudo cat /etc/docker/daemon.json | tee "$tmp/original.json" >/dev/null
else
    printf '{}\n' >"$tmp/original.json"
fi
render "$tmp/original.json" >"$tmp/daemon.json"
sudo dockerd --validate --config-file "$tmp/daemon.json"
sudo install -m 0600 "$tmp/daemon.json" /etc/docker/daemon.json
sudo systemctl restart docker
docker info --format '{{json .RegistryConfig.Mirrors}}' | jq -e 'index("https://mirror.gcr.io/") != null or index("https://mirror.gcr.io") != null' >/dev/null
echo 'Docker Hub public registry cache configured; immutable image pins retained'
