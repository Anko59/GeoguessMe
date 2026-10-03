#!/usr/bin/env bash
set -euo pipefail

readonly source_repo='https://github.com/FSDevelop/braces.git'
readonly source_branch='fix/limit-nesting-depth'
readonly source_commit='28d440b5dd449dbf1fe6f3506cf94ecca4d02660'
readonly destination='/workspace/frontend/vendor/braces'

if [[ -e "$destination" ]]; then
    echo "Refusing to overwrite $destination" >&2
    exit 1
fi

temp_dir=$(mktemp -d)
case "$temp_dir" in
    /tmp/tmp.*) ;;
    *)
        echo "Unexpected temporary path: $temp_dir" >&2
        exit 1
        ;;
esac

# The container is removed after this target, so its temporary clone needs no
# host-side cleanup. Pin both the branch and commit; fail if upstream moves it.
git clone --quiet --filter=blob:none --single-branch --branch "$source_branch" "$source_repo" "$temp_dir/source"
actual_commit=$(git -C "$temp_dir/source" rev-parse HEAD)
if [[ "$actual_commit" != "$source_commit" ]]; then
    echo "Expected braces backport $source_commit, found $actual_commit" >&2
    exit 1
fi

mkdir -p "$destination"
git -C "$temp_dir/source" archive --format=tar "$source_commit" index.js LICENSE package.json lib |
    tar -xf - -C "$destination"

BRACES_MANIFEST="$destination/package.json" node <<'NODE'
const fs = require('node:fs');
const file = process.env.BRACES_MANIFEST;
const manifest = JSON.parse(fs.readFileSync(file, 'utf8'));
manifest.version = '3.0.4+geoguessme.1';
manifest.private = true;
delete manifest.devDependencies;
delete manifest.scripts;
fs.writeFileSync(file, `${JSON.stringify(manifest, null, 2)}\n`);
NODE
