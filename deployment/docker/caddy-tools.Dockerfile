# syntax=docker/dockerfile:1
# Validation uses the same reviewed Caddy 2.11.7 runtime artifact as production.
# The caller must supply its immutable reference; do not fall back to an older
# upstream binary or compile a second, potentially different dependency graph.
ARG CADDY_RUNTIME_IMAGE
# The caller validates the immutable reference; a default would bypass that
# selection. Hadolint cannot resolve a mandatory image argument without a default.
# hadolint ignore=DL3006
FROM ${CADDY_RUNTIME_IMAGE}

ARG DEPENDENCY_INPUTS
LABEL dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}"
