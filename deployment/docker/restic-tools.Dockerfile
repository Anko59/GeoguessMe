# syntax=docker/dockerfile:1

# Temporary Restic 0.19.1 build from an asserted upstream source commit.
# Select the reviewed fixed module graph explicitly, rather than version-specific
# replacements that silently become inactive. Retire this when upstream ships
# a verified compatible artifact carrying the fixes.
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS restic-build

RUN apk add --no-cache git=2.54.0-r0
WORKDIR /src
RUN git clone --depth 1 --branch v0.19.1 https://github.com/restic/restic.git /src \
    && test "$(git rev-parse HEAD)" = 6aa3a516ce654808a1f28f9fa21e9b7c8e6e90bf \
    && go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.42.0 \
        google.golang.org/grpc@v1.83.2 golang.org/x/crypto@v0.57.0 \
    && go mod tidy \
    && go run build.go --output /out/restic

# The upstream restic/restic base (and every published alpine image so far)
# still ships libcrypto3 3.5.7-r0 with CVE-2026-14456; the fixed 3.5.8-r0
# exists only in the alpine package repositories. Per
# docs/security-scanning.md ("apply the fix in the shipped image"), the
# runtime is therefore pinned alpine with OpenSSL refreshed to at least the
# fixed release and pcre2 pinned to 10.49-r0; restic itself is a static binary,
# and the base already carries the CA trust store it needs for TLS.
FROM alpine:3.24@sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f
SHELL ["/bin/ash", "-o", "pipefail", "-c"]

RUN apk add --no-cache 'openssl>=3.5.8-r0' 'pcre2=10.49-r0' \
    && apk info -v | grep -Fxq 'pcre2-10.49-r0'
COPY --from=restic-build /out/restic /usr/bin/restic

ARG DEPENDENCY_INPUTS
LABEL dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}" \
    org.opencontainers.image.base.name="alpine:3.24" \
    org.opencontainers.image.base.digest="sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f"
