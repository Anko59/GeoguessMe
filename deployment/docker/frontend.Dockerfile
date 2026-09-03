# syntax=docker/dockerfile:1
# Production frontend gateway: builds the Vite SPA and serves it through Caddy,
# which also reverse-proxies the API and WebSocket endpoint same-origin.
# Node 22.23.2-alpine (immutable index digest).
FROM node:22.23.2-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Caddy 2.11.4 builder helper (immutable digest), using the separately pinned
# Go 1.26.6 compiler so the generated gateway binary includes current stdlib
# fixes as well as patched golang.org/x/net, golang.org/x/crypto, and gRPC
# modules.
FROM caddy:2.11.4-builder-alpine@sha256:8e89605351333ad2cc2f3bcc95275a2ccc427f88914050e86a5fde0fd77a63c4 AS xcaddy
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS caddy-build
RUN apk add --no-cache git=2.54.0-r0
COPY --from=xcaddy /usr/bin/xcaddy /usr/bin/xcaddy
RUN xcaddy build v2.11.4 \
    --output /usr/bin/caddy \
    --replace 'golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0' \
    --replace 'golang.org/x/crypto@v0.53.0=golang.org/x/crypto@v0.55.0' \
    --replace 'golang.org/x/crypto@v0.54.0=golang.org/x/crypto@v0.55.0' \
    --replace 'google.golang.org/grpc@v1.81.0=google.golang.org/grpc@v1.83.1'

# Caddy 2.11.4-alpine (immutable index digest), with the patched binary above.
FROM caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648
LABEL org.opencontainers.image.base.name="caddy:2.11.4-alpine" \
    org.opencontainers.image.base.digest="sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648"
COPY --from=caddy-build /usr/bin/caddy /usr/bin/caddy
COPY --from=build /app/frontend/dist /srv
COPY deployment/caddy/Caddyfile /etc/caddy/Caddyfile
RUN addgroup -S -g 1000 caddy \
    && adduser -S -D -H -u 1000 -G caddy caddy \
    && setcap cap_net_bind_service=+ep /usr/bin/caddy \
    && chown -R caddy:caddy /srv /data /config \
    && apk add --no-cache 'openssl>=3.5.8-r0'
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["wget", "--spider", "--quiet", "http://localhost/health/live"]
USER caddy
