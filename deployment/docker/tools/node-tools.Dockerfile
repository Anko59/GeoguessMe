# syntax=docker/dockerfile:1
# Node 22.23.2 bookworm-slim (linux/amd64 digest: d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436)
FROM node:22.23.2-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436

ARG TOOLS_UID=1000
ARG TOOLS_GID=1000

# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install --no-install-recommends -y bash git \
 && rm -rf /var/lib/apt/lists/* \
 && git config --system --add safe.directory /workspace \
 && if ! getent group "$TOOLS_GID" >/dev/null; then groupadd --gid "$TOOLS_GID" geoguessme-tools; fi \
 && if ! getent passwd "$TOOLS_UID" >/dev/null; then useradd --uid "$TOOLS_UID" --gid "$TOOLS_GID" --no-create-home --shell /usr/sbin/nologin geoguessme-tools; fi \
 && mkdir -p /workspace/frontend/node_modules

ENV npm_config_cache=/npm-cache \
    npm_config_update_notifier=false \
    PATH=/workspace/frontend/node_modules/.bin:$PATH

WORKDIR /workspace
