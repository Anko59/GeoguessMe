# syntax=docker/dockerfile:1

# Restic 0.19.1 with fixed Go modules. Upstream still embeds
# golang.org/x/net v0.55.0, golang.org/x/text v0.38.0, and
# google.golang.org/grpc v1.81.1, each with fixed HIGH findings.
# Build the pinned, signed source release with the fixed releases until
# upstream publishes an image that includes them.
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS restic-build

RUN apk add --no-cache git=2.54.0-r0
WORKDIR /src
RUN git clone --depth 1 --branch v0.19.1 https://github.com/restic/restic.git /src \
    && test "$(git rev-parse HEAD)" = 6aa3a516ce654808a1f28f9fa21e9b7c8e6e90bf \
    && go mod edit -replace=golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0 \
    && go get golang.org/x/text@v0.39.0 \
    && go get google.golang.org/grpc@v1.82.1 \
    && go mod tidy \
    && go run build.go --output /out/restic

# The upstream restic/restic base (and every published alpine image so far)
# still ships libcrypto3 3.5.7-r0 with CVE-2026-14456; the fixed 3.5.8-r0
# exists only in the alpine package repositories. Per
# docs/security-scanning.md ("apply the fix in the shipped image"), the
# runtime is therefore pinned alpine with the OpenSSL packages refreshed to at
# least the fixed release; restic itself is a static binary, and the base
# already carries the CA trust store it needs for TLS against object storage.
FROM alpine:3.24@sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f

RUN apk add --no-cache 'openssl>=3.5.8-r0'
COPY --from=restic-build /out/restic /usr/bin/restic

LABEL org.opencontainers.image.base.name="alpine:3.24" \
    org.opencontainers.image.base.digest="sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f"
