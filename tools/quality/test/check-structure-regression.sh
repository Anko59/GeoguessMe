#!/usr/bin/env bash
# Regression tests for the structure checker (tools/quality/structure-check).
set -euo pipefail

CHECKER="$(cd "$(dirname "$0")/.." && pwd)/structure-check"
PASS=0
FAIL=0

with_temp_repo() {
    local expected=$1
    local name=$2
    shift 2
    local directory
    directory=$(mktemp -d)
    local status=0
    (
        cd "$directory"
        git init -q
        git config user.email test@localhost
        git config user.name test
        "$@"
        git add -A
        git commit -q -m fixture
        STRUCTURE_REPO_ROOT="$directory" "$CHECKER" >/dev/null
    ) || status=$?
    rm -rf "$directory"
    if [ "$expected" = pass ] && [ "$status" -eq 0 ]; then
        PASS=$((PASS + 1))
    elif [ "$expected" = fail ] && [ "$status" -ne 0 ]; then
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name"
        FAIL=$((FAIL + 1))
    fi
}

echo "structure-check regression tests:"

# ── Positive (current repository) ────────────────────────────────────────────
if (cd "$(dirname "$CHECKER")/../.." && "$CHECKER" >/dev/null); then
    PASS=$((PASS + 1))
else
    echo "FAIL: current-repository"
    FAIL=$((FAIL + 1))
fi

# ── Line-count boundary ──────────────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass exact-500-line-limit bash -c 'for i in $(seq 1 500); do echo line; done > exact.md'
# shellcheck disable=SC2016
with_temp_repo fail exceeds-500-line-limit bash -c 'for i in $(seq 1 501); do echo line; done > too-long.md'

# ── Directory children boundary ──────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass exactly-14-code-children bash -c 'mkdir -p children-14; for i in $(seq 1 14); do echo x > "children-14/f$i.ts"; done'
# shellcheck disable=SC2016
with_temp_repo fail exceeds-14-code-children bash -c 'mkdir -p children-15; for i in $(seq 1 15); do echo x > "children-15/f$i.ts"; done'

# ── Directory with spaces in name ────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass spaces-in-dir-name bash -c 'mkdir -p "my dir"; for i in $(seq 1 14); do echo x > "my dir/f$i.ts"; done'

# ── Root-level special files ─────────────────────────────────────────────────
with_temp_repo pass root-dockerfile-makefile bash -c 'printf "FROM scratch\n" > Dockerfile; printf "x\n" > Makefile'

# ── Excluded lock files ──────────────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass package-lock-json-excluded bash -c 'for i in $(seq 1 501); do echo x; done > package-lock.json'

# shellcheck disable=SC2016
with_temp_repo pass go-sum-excluded bash -c 'for i in $(seq 1 501); do echo x; done > go.sum'

# ── Vendor exclusion ─────────────────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass vendor-directory-excluded bash -c 'mkdir -p vendor; for i in $(seq 1 501); do echo x; done > vendor/external.go'

# ── Binary media exclusion ───────────────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass binary-png-excluded bash -c 'for i in $(seq 1 501); do echo x; done > image.png'
# shellcheck disable=SC2016
with_temp_repo pass binary-jpg-excluded bash -c 'for i in $(seq 1 501); do echo x; done > photo.jpg'
# shellcheck disable=SC2016
with_temp_repo pass binary-svg-excluded bash -c 'for i in $(seq 1 501); do echo x; done > icon.svg'
# shellcheck disable=SC2016
with_temp_repo pass binary-webp-excluded bash -c 'for i in $(seq 1 501); do echo x; done > anim.webp'
# shellcheck disable=SC2016
with_temp_repo pass binary-woff2-excluded bash -c 'for i in $(seq 1 501); do echo x; done > font.woff2'

# ── Non-code children are not counted toward the 14-child limit ──────────────
# shellcheck disable=SC2016
with_temp_repo pass non-code-children-not-counted bash -c 'mkdir -p mixed; for i in $(seq 1 20); do echo x > "mixed/doc$i.md"; done; for i in $(seq 1 14); do echo x > "mixed/f$i.ts"; done'
# 20 .md files and 14 .ts files — only .ts counts, so 14 <= 14 passes

# ── Only code/config files are counted for children limit ────────────────────
# shellcheck disable=SC2016
with_temp_repo pass only-code-children-counted bash -c 'mkdir -p many-txt; for i in $(seq 1 30); do echo x > "many-txt/doc$i.txt"; done'
# 30 .txt files — not code/config, so 0 counted. The directory still exists.
# shellcheck disable=SC2016
with_temp_repo pass root-code-only-counted bash -c 'for i in $(seq 1 20); do echo x > "readme$i.md"; done'

# ── Deep nesting clears parent check ─────────────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo pass nested-children-not-flat bash -c 'mkdir -p deep/a deep/b deep/c; for i in $(seq 1 14); do echo x > "deep/a/f$i.ts"; done; for i in $(seq 1 14); do echo x > "deep/b/f$i.ts"; done; for i in $(seq 1 14); do echo x > "deep/c/f$i.ts"; done'
# deep/a, deep/b, deep/c each have 14 files — each passes. deep/ itself has 3 dirs (3 children) — passes.

# ── Mixed code/config types counted together ─────────────────────────────────
# shellcheck disable=SC2016
with_temp_repo fail mixed-extensions-exceed-count bash -c 'mkdir -p mixed-ext; for i in $(seq 1 5); do echo x > "mixed-ext/f$i.go"; done; for i in $(seq 1 5); do echo x > "mixed-ext/f$i.ts"; done; for i in $(seq 1 5); do echo x > "mixed-ext/f$i.yaml"; done'
# 15 code/config files total (5 .go + 5 .ts + 5 .yaml) — exceeds 14

# ── Generated-file marker: syntax-aware ───────────────────────────────────────
#
# pass  = small file with a valid language-appropriate marker → excluded, no violation
# fail  = 501-line file with invalid/bare marker           → NOT excluded, line-count violation

# shellcheck disable=SC2016
with_temp_repo pass go-line-comment-marker bash -c 'mkdir -p tools/quality && printf "// GENERATED FILE\npackage main\n" > main.go && printf "main.go\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail go-bare-marker bash -c 'mkdir -p tools/quality && printf "GENERATED FILE\n" > main.go && for i in $(seq 1 500); do echo line; done >> main.go && printf "main.go\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass go-block-comment-marker bash -c 'mkdir -p tools/quality && printf "/* GENERATED FILE */\npackage main\n" > main.go && printf "main.go\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass ts-line-comment-marker bash -c 'mkdir -p tools/quality && printf "// GENERATED FILE\nexport const x = 1;\n" > lib.ts && printf "lib.ts\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass yaml-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nkey: value\n" > config.yaml && printf "config.yaml\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail yaml-bare-marker bash -c 'mkdir -p tools/quality && printf "GENERATED FILE\n" > config.yaml && for i in $(seq 1 500); do echo line; done >> config.yaml && printf "config.yaml\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass sql-dash-marker bash -c 'mkdir -p tools/quality && printf "%s\n%s\n" "-- GENERATED FILE" "CREATE TABLE t (id int);" > schema.sql && printf "schema.sql\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass html-comment-marker bash -c 'mkdir -p tools/quality && printf "<!-- GENERATED FILE -->\n<!DOCTYPE html>\n" > index.html && printf "index.html\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail html-bare-marker bash -c 'mkdir -p tools/quality && printf "GENERATED FILE\n" > index.html && for i in $(seq 1 500); do echo line; done >> index.html && printf "index.html\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass css-block-comment-marker bash -c 'mkdir -p tools/quality && printf "/* GENERATED FILE */\nbody {}\n" > style.css && printf "style.css\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail css-line-comment-rejected bash -c 'mkdir -p tools/quality && printf "// GENERATED FILE\n" > style.css && for i in $(seq 1 500); do echo line; done >> style.css && printf "style.css\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass json-key-marker bash -c 'mkdir -p tools/quality && printf "{\"_generated\": \"GENERATED FILE\"}\n{}\n" > config.json && printf "config.json\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail json-bare-marker bash -c 'mkdir -p tools/quality && printf "GENERATED FILE\n" > config.json && for i in $(seq 1 500); do echo line; done >> config.json && printf "config.json\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass non-code-bare-marker bash -c 'mkdir -p tools/quality && printf "GENERATED FILE\nSome text\n" > readme.md && printf "readme.md\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail allowlist-without-marker bash -c 'mkdir -p tools/quality && printf "package main\n" > main.go && for i in $(seq 1 500); do echo line; done >> main.go && printf "main.go\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo fail spoofed-malformed-marker bash -c 'mkdir -p tools/quality && printf " GENERATED FILE\n" > main.go && for i in $(seq 1 500); do echo line; done >> main.go && printf "main.go\n" > tools/quality/generated-files.allowlist'

# shellcheck disable=SC2016
with_temp_repo pass toml-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nkey = \"value\"\n" > config.toml && printf "config.toml\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass ini-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nkey=value\n" > config.ini && printf "config.ini\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass env-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nKEY=value\n" > .env && printf ".env\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass dockerfile-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nFROM scratch\n" > Dockerfile && printf "Dockerfile\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass makefile-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nall:\n\t@echo done\n" > Makefile && printf "Makefile\n" > tools/quality/generated-files.allowlist'
# shellcheck disable=SC2016
with_temp_repo pass caddyfile-hash-marker bash -c 'mkdir -p tools/quality && printf "# GENERATED FILE\nlocalhost\n" > Caddyfile && printf "Caddyfile\n" > tools/quality/generated-files.allowlist'

echo "results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
