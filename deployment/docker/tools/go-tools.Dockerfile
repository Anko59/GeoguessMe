# syntax=docker/dockerfile:1
# Go 1.26.6-alpine (immutable index digest: af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df)
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df

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
