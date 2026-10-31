# syntax=docker/dockerfile:1
# Go 1.27.3-alpine (immutable index digest: d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb)
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb

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
