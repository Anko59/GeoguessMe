# syntax=docker/dockerfile:1

# LinuxServer socket-proxy 3.4.6 still ships Alpine pcre2 10.48-r0, which is
# affected by CVE-2026-103111. Upgrade only that runtime package from the same
# signed Alpine repository while preserving the upstream entrypoint and user.
FROM lscr.io/linuxserver/socket-proxy:3.4.6@sha256:0357c479cc98e863917d1cd8b10e83d35a50ca46a0788f5a192bf792c3b7100d
SHELL ["/bin/ash", "-o", "pipefail", "-c"]

ARG DEPENDENCY_INPUTS

RUN apk add --no-cache --upgrade 'pcre2=10.49-r0' \
    && apk info -v | grep -Fxq 'pcre2-10.49-r0'

LABEL org.opencontainers.image.source="https://github.com/Anko59/GeoguessMe" \
    org.opencontainers.image.description="LinuxServer socket-proxy 3.4.6 with fixed PCRE2 10.49" \
    org.opencontainers.image.version="3.4.6-pcre2-10.49-r0" \
    dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}" \
    org.opencontainers.image.base.name="lscr.io/linuxserver/socket-proxy:3.4.6" \
    org.opencontainers.image.base.digest="sha256:0357c479cc98e863917d1cd8b10e83d35a50ca46a0788f5a192bf792c3b7100d"
