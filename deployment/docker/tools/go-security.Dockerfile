# syntax=docker/dockerfile:1
# Go 1.26.9-alpine (immutable index digest: cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0)
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0

# Specialized security and operations tools: vulnerability scanning, race
# detection (CGO), database client utilities. Normal format/lint/test/build
# operations use the smaller go-tools image.
# hadolint ignore=DL3018
RUN apk add --no-cache bash build-base curl git jq postgresql-client \
 && git config --system --add safe.directory /workspace

ENV CGO_ENABLED=1 \
    GOPATH=/go \
    GOTOOLCHAIN=local

RUN go install golang.org/x/vuln/cmd/govulncheck@v1.1.4

WORKDIR /workspace
