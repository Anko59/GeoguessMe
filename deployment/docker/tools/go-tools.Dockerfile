# syntax=docker/dockerfile:1
# Go 1.25.12-alpine (linux/amd64 digest: 56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587)
FROM golang:1.25.12-alpine@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587

# The base image is immutable; Alpine package indexes are intentionally used
# for the small runtime tool set and are covered by the tool-image digest.
# hadolint ignore=DL3018
RUN apk add --no-cache bash build-base curl git postgresql-client \
 && git config --system --add safe.directory /workspace

ENV CGO_ENABLED=1 \
    GOPATH=/go \
    GOTOOLCHAIN=local

# Versions are deliberately pinned. They are updated as a single tool-image
# change so local development and CI use the same analyzers.
RUN go install golang.org/x/tools/cmd/goimports@v0.30.0 \
 && go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8 \
 && go install golang.org/x/vuln/cmd/govulncheck@v1.1.4

WORKDIR /workspace
