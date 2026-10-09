# syntax=docker/dockerfile:1
# Temporary released Caddy build: retire when a verified official digest carries
# the same fixes. This artifact is independent of the application's Vite bundle.
FROM caddy:2.11.4-builder-alpine@sha256:8e89605351333ad2cc2f3bcc95275a2ccc427f88914050e86a5fde0fd77a63c4 AS xcaddy
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS build
RUN apk add --no-cache git=2.54.0-r0
COPY --from=xcaddy /usr/bin/xcaddy /usr/bin/xcaddy
RUN xcaddy build v2.11.7 --output /usr/bin/caddy

FROM caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
COPY --from=build /usr/bin/caddy /usr/bin/caddy
RUN apk add --no-cache 'openssl>=3.5.8-r0' \
        'curl>=8.22.0-r0' 'libcurl>=8.22.0-r0' \
        'c-ares>=1.34.8-r0' 'pcre2=10.49-r0' \
    && apk info -v | grep -Fxq 'pcre2-10.49-r0' \
    && caddy version | grep -Fq 'v2.11.7' \
    && setcap cap_net_bind_service=+ep /usr/bin/caddy
ARG DEPENDENCY_INPUTS
LABEL org.opencontainers.image.base.name="caddy:2.11.4-alpine" \
    org.opencontainers.image.base.digest="sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648" \
    dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}"
