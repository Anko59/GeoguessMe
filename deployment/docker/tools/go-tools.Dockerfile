# syntax=docker/dockerfile:1
# Go 1.25.13-alpine (immutable index digest: 844b27705f54e73773e0f9bc3c780633b9d7f4b4831bf35cdad02a81a4c80bd0)
FROM golang:1.25.13-alpine@sha256:844b27705f54e73773e0f9bc3c780633b9d7f4b4831bf35cdad02a81a4c80bd0

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
