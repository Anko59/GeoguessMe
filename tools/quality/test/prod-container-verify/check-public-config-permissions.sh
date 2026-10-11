#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/../../../.." && pwd)"
helper="$root/deployment/oauth2-proxy/prepare-public-configs.sh"
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
mkdir -p "$fixture/deployment/oauth2-proxy" "$fixture/deployment/env"
cp "$root/deployment/oauth2-proxy/"* "$fixture/deployment/oauth2-proxy/"
printf 'fake-private-sentinel\n' >"$fixture/deployment/env/private.env"
chmod 0600 "$fixture/deployment/oauth2-proxy/"* "$fixture/deployment/env/private.env"
bash "$helper" "$fixture"
for name in oauth2-proxy.cfg oauth2-proxy-alpha.yaml; do
    test "$(stat -c %a "$fixture/deployment/oauth2-proxy/$name")" = 644
    cmp "$root/deployment/oauth2-proxy/$name" "$fixture/deployment/oauth2-proxy/$name"
done
test "$(stat -c %a "$fixture/deployment/env/private.env")" = 600
test "$(<"$fixture/deployment/env/private.env")" = fake-private-sentinel
# Hosted current/previous checkout aliases are legitimate roots.
ln -s "$fixture" "$fixture/current"
chmod 0600 "$fixture/deployment/oauth2-proxy/oauth2-proxy.cfg" \
    "$fixture/deployment/oauth2-proxy/oauth2-proxy-alpha.yaml"
bash "$helper" "$fixture/current"
test "$(stat -c %a "$fixture/deployment/oauth2-proxy/oauth2-proxy.cfg")" = 644
# Refuse symlinks, including when the first public file would otherwise be valid.
for name in oauth2-proxy.cfg oauth2-proxy-alpha.yaml; do
    path="$fixture/deployment/oauth2-proxy/$name"
    test "$path" = "$fixture/deployment/oauth2-proxy/oauth2-proxy.cfg" ||
        test "$path" = "$fixture/deployment/oauth2-proxy/oauth2-proxy-alpha.yaml"
    rm "$path"
    ln -s ../env/private.env "$path"
    if bash "$helper" "$fixture" >/dev/null 2>&1; then
        echo 'FAIL: public config normalizer followed a private-file symlink' >&2
        exit 1
    fi
    test "$(stat -c %a "$fixture/deployment/env/private.env")" = 600
    rm "$path"
    cp "$root/deployment/oauth2-proxy/$name" "$path"
done
# Missing files and symlinked config directories also fail closed.
path="$fixture/deployment/oauth2-proxy/oauth2-proxy-alpha.yaml"
test "$path" = "$fixture/deployment/oauth2-proxy/oauth2-proxy-alpha.yaml"
rm "$path"
if bash "$helper" "$fixture" >/dev/null 2>&1; then exit 1; fi
mv "$fixture/deployment/oauth2-proxy" "$fixture/deployment/public-target"
ln -s public-target "$fixture/deployment/oauth2-proxy"
if bash "$helper" "$fixture" >/dev/null 2>&1; then exit 1; fi
for caller in "$root/deployment/scripts/prod-container-verify.sh" "$root/tools/make/deployment.mk"; do
    grep -q 'prepare-public-configs.sh' "$caller"
done
grep -Fq "prepare_public_configs \"\$release\"" "$root/deployment/scripts/hosted/deploy.sh"
# Inspect only the canonical dev-social recipe; never start its live services.
awk '
    /^dev-social:/ { inside = 1; next }
    inside && /^[^[:space:]]/ { exit }
    inside && /deployment\/oauth2-proxy\/prepare-public-configs.sh/ { preparer = NR }
    inside && /\$\(COMPOSE_DEV\).*up / && !first_up { first_up = NR }
    END { if (!preparer || !first_up || preparer >= first_up) exit 1 }
' "$root/tools/make/setup.mk"
# The installed helper must work on old releases with only the two templates.
# No preparer script or common.sh is present inside this fake release checkout.
mkdir -p "$fixture/older/deployment/oauth2-proxy"
for name in oauth2-proxy.cfg oauth2-proxy-alpha.yaml; do
    cp "$root/deployment/oauth2-proxy/$name" "$fixture/older/deployment/oauth2-proxy/$name"
    chmod 0600 "$fixture/older/deployment/oauth2-proxy/$name"
done
sh -c '. "$1"; prepare_public_configs "$2"' _ \
    "$root/deployment/scripts/hosted/common.sh" "$fixture/older"
test "$(stat -c %a "$fixture/older/deployment/oauth2-proxy/oauth2-proxy.cfg")" = 644
echo 'Public config permissions PASSED: restrictive checkout, unchanged content/private files, symlink/missing guards, canonical callers'
