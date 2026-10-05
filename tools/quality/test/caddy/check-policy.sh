#!/bin/sh
set -eu

policy=$(sed -n 's/^[[:space:]]*Content-Security-Policy "\(.*\)"$/\1/p' /workspace/deployment/caddy/Caddyfile)
if [ -z "$policy" ]; then
    printf '%s\n' 'Caddy CSP policy is missing' >&2
    exit 1
fi

require_source() {
    directive=$1
    origin=$2
    if ! printf '%s\n' "$policy" | tr ';' '\n' | awk -v directive="$directive" -v origin="$origin" '
		$1 == directive {
			for (i = 2; i <= NF; i++) {
				if ($i == origin) found = 1
			}
		}
		END { exit !found }
	'; then
        printf 'Caddy CSP %s is missing %s\n' "$directive" "$origin" >&2
        exit 1
    fi
}

require_source img-src https://tile.openstreetmap.org
require_source connect-src https://tile.openstreetmap.org
require_source connect-src https://gibs.earthdata.nasa.gov

# COPY preserves checkout modes, including a private umask's 0600. The
# root-owned public config must be readable by the final non-root gateway.
require_readable_config() {
    if ! grep -Fq 'chmod 0644 /etc/caddy/Caddyfile' "$1"; then
        printf '%s\n' 'Caddy image must explicitly install readable public configuration' >&2
        return 1
    fi
}

dockerfile=/workspace/deployment/docker/frontend.Dockerfile
require_readable_config "$dockerfile"
# This ephemeral-container fixture proves removing the installation permission
# command fails the policy rather than merely checking the current happy path.
fixture=$(mktemp)
sed '/chmod 0644 \/etc\/caddy\/Caddyfile/d' "$dockerfile" >"$fixture"
if require_readable_config "$fixture" >/dev/null 2>&1; then
    printf '%s\n' 'Caddy unreadable-configuration regression fixture was accepted' >&2
    exit 1
fi

printf '%s\n' 'Caddy globe tile CSP and image configuration permission policy passed'
