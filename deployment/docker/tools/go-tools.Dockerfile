# syntax=docker/dockerfile:1
# Go 1.27.2-alpine (immutable index digest: 85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673)
FROM golang:1.27.2-alpine@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673

# Lightweight operations tooling: formatting, linting, testing, building.
# Heavy security/ops tools (govulncheck, postgresql-client, CGO build chain)
# are isolated in the separate go-security image.
# hadolint ignore=DL3018
RUN apk add --no-cache bash curl git \
 && git config --system --add safe.directory /workspace

ENV CGO_ENABLED=0 \
    GOPATH=/go \
    GOTOOLCHAIN=local

# Versions are deliberately pinned. They are updated as a single tool-image
# change so local development and CI use the same analyzers.
RUN go install golang.org/x/tools/cmd/goimports@v0.50.0 \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

WORKDIR /workspace
