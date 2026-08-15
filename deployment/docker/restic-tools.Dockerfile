# syntax=docker/dockerfile:1

# Restic 0.19.1 with the fixed golang.org/x/net module. The upstream image
# still embeds v0.55.0, so build the pinned, signed source release with v0.56.0
# until an upstream image includes the fix.
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS restic-build

RUN apk add --no-cache git=2.54.0-r0
WORKDIR /src
RUN git clone --depth 1 --branch v0.19.1 https://github.com/restic/restic.git /src \
    && test "$(git rev-parse HEAD)" = 6aa3a516ce654808a1f28f9fa21e9b7c8e6e90bf \
    && go mod edit -replace=golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0 \
    && go mod download golang.org/x/net \
    && go mod tidy \
    && go run build.go --output /out/restic

FROM restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510
LABEL org.opencontainers.image.base.name="restic/restic:0.19.1" \
    org.opencontainers.image.base.digest="sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510"
COPY --from=restic-build /out/restic /usr/bin/restic
