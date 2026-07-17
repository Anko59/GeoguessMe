# syntax=docker/dockerfile:1
# Production frontend gateway: builds the Vite SPA and serves it through Caddy,
# which also reverse-proxies the API and WebSocket endpoint same-origin.
# Node 22-alpine (immutable index digest).
FROM node:22-alpine@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2 AS build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Caddy 2.10-alpine (immutable index digest).
FROM caddy:2.10-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
COPY --from=build /app/frontend/dist /srv
COPY deployment/caddy/Caddyfile /etc/caddy/Caddyfile
RUN addgroup -S -g 1000 caddy \
    && adduser -S -D -H -u 1000 -G caddy caddy \
    && chown -R caddy:caddy /srv /data /config
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["wget", "--spider", "--quiet", "http://localhost/health/live"]
USER caddy
