# syntax=docker/dockerfile:1
# Production frontend gateway: builds the Vite SPA and serves it through Caddy,
# which also reverse-proxies the API and WebSocket endpoint same-origin.
FROM node:22-alpine AS build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN cd frontend && npm ci
COPY frontend/ ./frontend/
RUN cd frontend && npm run build

FROM caddy:2.10-alpine
COPY --from=build /app/frontend/dist /srv
COPY deployment/caddy/Caddyfile /etc/caddy/Caddyfile
EXPOSE 80
