# syntax=docker/dockerfile:1
# Production gateway uses the already-built security-reviewed Caddy artifact.
# Make supplies a verified config-ID-addressed alias; CI a signed registry digest.
ARG CADDY_RUNTIME_IMAGE
FROM node:22.23.5-alpine@sha256:1314fb30ec37592d5845e5a160d522c0e680972aa534549940b8d73bde88481a AS build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
COPY frontend/vendor/braces-security/ ./vendor/braces-security/
RUN npm ci
COPY frontend/ ./
RUN npm run build

# The required input is validated by the dependency lifecycle, never a default tag.
# hadolint ignore=DL3006
FROM ${CADDY_RUNTIME_IMAGE}
ARG CADDY_RUNTIME_IMAGE
LABEL dev.geoguessme.caddy-runtime="${CADDY_RUNTIME_IMAGE}"
COPY --from=build /app/frontend/dist /srv
COPY deployment/caddy/Caddyfile /etc/caddy/Caddyfile
RUN addgroup -S -g 1000 caddy \
    && adduser -S -D -H -u 1000 -G caddy caddy \
    && chown -R caddy:caddy /srv /data /config \
    && chmod 0644 /etc/caddy/Caddyfile
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["wget", "--spider", "--quiet", "http://localhost/health/live"]
USER caddy
