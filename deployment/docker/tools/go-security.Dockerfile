# syntax=docker/dockerfile:1
# Go 1.26.6-alpine (immutable index digest: af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df)
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df

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
