# syntax=docker/dockerfile:1
# Go 1.27.2-alpine (immutable index digest: 85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673)
FROM golang:1.27.2-alpine@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673

# Specialized security and operations tools: vulnerability scanning, race
# detection (CGO), database client utilities. Normal format/lint/test/build
# operations use the smaller go-tools image.
# hadolint ignore=DL3018
RUN apk add --no-cache bash build-base curl git jq postgresql-client \
 && git config --system --add safe.directory /workspace

ENV CGO_ENABLED=1 \
    GOPATH=/go \
    GOTOOLCHAIN=local

RUN go install golang.org/x/vuln/cmd/govulncheck@v1.8.0

WORKDIR /workspace
