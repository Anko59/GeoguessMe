# syntax=docker/dockerfile:1
# Node 22.14.0 bookworm-slim (linux/amd64 digest: 745403dc46b5ab4c998502b07a12cbf020cf2c30645427a68ec0718f02d647de)
FROM node:22.14.0-bookworm-slim@sha256:1c18d9ab3af4585870b92e4dbc5cac5a0dc77dd13df1a5905cea89fc720eb05b

# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install --no-install-recommends -y bash git \
 && rm -rf /var/lib/apt/lists/* \
 && git config --system --add safe.directory /workspace \
 && mkdir -p /workspace/frontend/node_modules

ENV npm_config_cache=/npm-cache \
    npm_config_update_notifier=false \
    PATH=/workspace/frontend/node_modules/.bin:$PATH

WORKDIR /workspace
