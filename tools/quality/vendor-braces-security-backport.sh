#!/usr/bin/env bash
set -euo pipefail

readonly source_repo='https://github.com/micromatch/braces.git'
readonly source_ref='refs/pull/72/head'
readonly source_commit='28d440b5dd449dbf1fe6f3506cf94ecca4d02660'
readonly vendor_root='/workspace/frontend/vendor'
destination=${BRACES_BACKPORT_DESTINATION:-$vendor_root/braces}

if [[ "${destination%/*}" != "$vendor_root" ]]; then
    echo "Destination must be a direct child of $vendor_root: $destination" >&2
    exit 1
fi
case "${destination##*/}" in
    braces | braces-smoke-*) ;;
    *)
        echo "Destination must be braces or a braces-smoke-* path: $destination" >&2
        exit 1
        ;;
esac
if [[ -e "$destination" ]]; then
    echo "Refusing to overwrite $destination" >&2
    exit 1
fi

mkdir -p "$vendor_root"
staging_dir=$(mktemp -d "$vendor_root/.braces-backport.XXXXXX")
case "$staging_dir" in
    "$vendor_root"/.braces-backport.*) ;;
    *)
        echo "Unexpected staging path: $staging_dir" >&2
        exit 1
        ;;
esac
cleanup() {
    if [[ -n ${staging_dir:-} ]]; then
        case "$staging_dir" in
            "$vendor_root"/.braces-backport.*) rm -rf -- "$staging_dir" ;;
            *) echo "Refusing to clean unexpected staging path: $staging_dir" >&2 ;;
        esac
    fi
}
trap cleanup EXIT

# Node includes its Mozilla root bundle; expose it to Git so the node-tools image
# can validate HTTPS without disabling certificate checks or requiring host CAs.
temp_dir=$(mktemp -d)
case "$temp_dir" in
    /tmp/tmp.*) ;;
    *)
        echo "Unexpected temporary path: $temp_dir" >&2
        exit 1
        ;;
esac
ca_file="$temp_dir/node-roots.pem"
NODE_CA_FILE="$ca_file" node <<'NODE'
const fs = require('node:fs');
const tls = require('node:tls');
fs.writeFileSync(process.env.NODE_CA_FILE, `${tls.rootCertificates.join('\n')}\n`);
NODE

# Fetch the PR ref from the canonical repository, then verify the immutable pin.
# The ref may move; a moved ref fails closed instead of importing newer code.
git init --quiet "$temp_dir/source"
GIT_SSL_CAINFO="$ca_file" git -C "$temp_dir/source" fetch --quiet --depth=1 "$source_repo" "$source_ref"
git -C "$temp_dir/source" checkout --quiet --detach FETCH_HEAD
actual_commit=$(git -C "$temp_dir/source" rev-parse HEAD)
if [[ "$actual_commit" != "$source_commit" ]]; then
    echo "Expected braces backport $source_commit, found $actual_commit" >&2
    exit 1
fi

package_dir="$staging_dir/package"
mkdir "$package_dir"
git -C "$temp_dir/source" archive --format=tar "$source_commit" index.js LICENSE package.json lib |
    tar -xf - -C "$package_dir"
BRACES_MANIFEST="$package_dir/package.json" node <<'NODE'
const fs = require('node:fs');
const file = process.env.BRACES_MANIFEST;
const manifest = JSON.parse(fs.readFileSync(file, 'utf8'));
manifest.version = '3.0.4+geoguessme.1';
manifest.private = true;
delete manifest.devDependencies;
delete manifest.scripts;
fs.writeFileSync(file, `${JSON.stringify(manifest, null, 2)}\n`);
NODE
cat >"$package_dir/README.geoguessme.md" <<'README'
# GeoGuessMe braces security backport

This private package copy preserves `braces` 3.x compatibility while applying
upstream PR [micromatch/braces#72](https://github.com/micromatch/braces/pull/72)
at the pinned source commit `28d440b5dd449dbf1fe6f3506cf94ecca4d02660`. It adds
bounded nesting checks to parsing and AST walkers to address
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), plus
the follow-up compatibility fixes included in that commit.

The local package version `3.0.4+geoguessme.1` identifies this patched private
copy; it is not an upstream npm release. The `vendor-braces-security-backport`
Make target fetches the upstream PR head ref, verifies the exact source commit,
and fails closed if that ref moves. It writes only to an absent destination and
regenerates this provenance note. The dependency is covered by the focused
Vitest security/compatibility tests under `frontend/src/utils/bracesSecurity/`;
`make lint-css` also exercises Stylelint's globbing consumers.

Remove this package and its local devDependency after upstream publishes a
patched release for the issue tracked at
[braces issue #73](https://github.com/micromatch/braces/issues/73).
README

# Install only after every import and manifest operation succeeds so retries
# never inherit a partial destination. The destination was validated above.
if [[ -e "$destination" ]]; then
    echo "Refusing to overwrite $destination" >&2
    exit 1
fi
mv -- "$package_dir" "$destination"
rmdir "$staging_dir"
staging_dir=''
