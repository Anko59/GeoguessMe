#!/usr/bin/env bash
set -euo pipefail

# Verify the locally built production images expose the hardening properties
# that are otherwise easy to lose during a Dockerfile refactor.
backend_image="${BACKEND_IMAGE:-${LOCAL_BACKEND_IMAGE:?Run through Make or set BACKEND_IMAGE}}"
web_image="${WEB_IMAGE:-${LOCAL_WEB_IMAGE:?Run through Make or set WEB_IMAGE}}"
# Freeze both selected references before hardening checks or Compose expansion.
# A mutable local tag can move, but these content-addressed IDs cannot.
backend_image="$(docker image inspect --format '{{.Id}}' "$backend_image")"
web_image="$(docker image inspect --format '{{.Id}}' "$web_image")"
if [[ ! "$backend_image" =~ ^sha256:[a-f0-9]{64}$ || ! "$web_image" =~ ^sha256:[a-f0-9]{64}$ ]]; then
    echo 'Docker returned an invalid application image ID' >&2
    exit 2
fi

for image in "$backend_image" "$web_image"; do
    docker image inspect "$image" >/dev/null
    user="$(docker image inspect --format '{{.Config.User}}' "$image")"
    test -n "$user" || {
        echo "$image has no explicit non-root user" >&2
        exit 1
    }
    case "$user" in
        0 | root | 0:0 | root:root)
            echo "$image runs as root" >&2
            exit 1
            ;;
    esac
    health="$(docker image inspect --format '{{if .Config.Healthcheck}}{{.Config.Healthcheck.Test}}{{end}}' "$image")"
    test -n "$health" || {
        echo "$image has no image healthcheck" >&2
        exit 1
    }
done

BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image" \
    docker compose -f deployment/compose.production.yaml --project-directory . config --quiet

echo "container-verify PASSED: non-root users, image healthchecks, and production Compose configuration verified"
