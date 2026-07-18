# syntax=docker/dockerfile:1
# Go 1.25.12-alpine (linux/amd64 digest: 56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587)
FROM golang:1.25.12-alpine@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587

# Specialized security and operations tools: vulnerability scanning, race
# detection (CGO), database client utilities. Normal format/lint/test/build
# operations use the smaller go-tools image.
# hadolint ignore=DL3018
RUN apk add --no-cache bash build-base curl git postgresql-client \
 && git config --system --add safe.directory /workspace

ENV CGO_ENABLED=1 \
    GOPATH=/go \
    GOTOOLCHAIN=local

RUN go install golang.org/x/vuln/cmd/govulncheck@v1.1.4

WORKDIR /workspace
