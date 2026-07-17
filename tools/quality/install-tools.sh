#!/usr/bin/env bash
# Install development quality tools. Run once or add missing tools to PATH.
# Some tools require Go, Node, Python, or system package managers.
set -euo pipefail
echo "Installing quality tools for GeoGuessMe..."
echo
echo "=== Go tools ==="
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest 2>/dev/null || echo "  skip (go not available)"
echo "=== Node tools ==="
cd "$(dirname "$0")/../../frontend"
npx lefthook install 2>/dev/null || echo "  lefthook: skip"
echo "=== Shell ==="
echo "  shellcheck: brew install shellcheck / apt install shellcheck / see https://github.com/koalaman/shellcheck"
echo "  shfmt:       go install mvdan.cc/sh/v3/cmd/shfmt@latest"
echo "=== Docker ==="
echo "  hadolint:    brew install hadolint / see https://github.com/hadolint/hadolint"
echo "=== Actions ==="
echo "  actionlint:  go install github.com/rhysd/actionlint/cmd/actionlint@latest"
echo "=== SQL ==="
echo "  sqlfluff:    pip install sqlfluff"
echo "=== Caddy ==="
echo "  caddy:       brew install caddy / see https://caddyserver.com/docs/install"
echo "=== OpenAPI ==="
echo "  redocly:     npx @redocly/cli lint docs/openapi.yaml"
echo
echo "Done. Re-run './tools/quality/install-tools.sh' to install missing tools."
