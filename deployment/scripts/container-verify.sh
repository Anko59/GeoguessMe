#!/usr/bin/env bash
set -euo pipefail

# Verify the locally built production images expose the hardening properties
# that are otherwise easy to lose during a Dockerfile refactor.
backend_image="${BACKEND_IMAGE:-geoguessme-backend:local}"
web_image="${WEB_IMAGE:-geoguessme-web:local}"

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
