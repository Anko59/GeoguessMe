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

printf '%s\n' 'Caddy globe tile CSP policy passed'
