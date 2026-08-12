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

# Caddy 2.11.4 builder (immutable index digest). The released Caddy image still
# carries golang.org/x/net v0.55.0, which has a fixed High vulnerability. Build
# the same Caddy release with the patched v0.56.0 module until upstream ships a
# patched image.
FROM caddy:2.11.4-builder-alpine@sha256:8e89605351333ad2cc2f3bcc95275a2ccc427f88914050e86a5fde0fd77a63c4 AS caddy-build
RUN xcaddy build v2.11.4 \
    --output /usr/bin/caddy \
    --replace 'golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0'

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
    && chown -R caddy:caddy /srv /data /config
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["wget", "--spider", "--quiet", "http://localhost/health/live"]
USER caddy
