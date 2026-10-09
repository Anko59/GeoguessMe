# syntax=docker/dockerfile:1
# Go 1.26.9-alpine (immutable index digest: cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0)
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0

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
RUN go install golang.org/x/tools/cmd/goimports@v0.30.0 \
 && go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8

WORKDIR /workspace
