# syntax=docker/dockerfile:1

# Cloudflared 2026.9.3 retains libssl3t64 deb13u2. Debian's deb13u3 fixes
# CVE-2026-75804 and CVE-2026-84782 without replacing the upstream binary.
# Both stages must target AMD64; the pinned upstream runtime is AMD64-only.
FROM debian:trixie@sha256:d5ce19d4736f0ebbacd686d1040271a5aeb0cc920f5990c1bfae1717627f0674 AS packages
WORKDIR /tmp
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# Default supports the legacy builder; native/package assertions still reject ARM.
ARG TARGETARCH=amd64
ARG LIBSSL_VERSION=3.5.7-1~deb13u3
RUN test "$TARGETARCH" = amd64 \
    && test "$(dpkg --print-architecture)" = amd64 \
    && apt-get update -qq \
    && apt-get download "libssl3t64=${LIBSSL_VERSION}" \
    && test "$(dpkg-deb -f libssl3t64_*.deb Package)" = libssl3t64 \
    && test "$(dpkg-deb -f libssl3t64_*.deb Version)" = "$LIBSSL_VERSION" \
    && test "$(dpkg-deb -f libssl3t64_*.deb Architecture)" = amd64 \
    && dpkg-deb -x libssl3t64_*.deb /tmp/extract \
    && dpkg-deb -f libssl3t64_*.deb > /tmp/extract-status \
    && printf 'Status: install ok installed\n' >> /tmp/extract-status

FROM cloudflare/cloudflared:2026.9.3-amd64@sha256:2fa795d0271a71c133a8f17c19a2a625e1976ad716329e0e61789f9fd8ee0091
ARG DEPENDENCY_INPUTS
LABEL dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}" \
    org.opencontainers.image.version="2026.9.3-openssl-deb13u3" \
    org.opencontainers.image.source="https://github.com/Anko59/GeoguessMe"
LABEL org.opencontainers.image.base.name="cloudflare/cloudflared:2026.9.3-amd64" \
    org.opencontainers.image.base.digest="sha256:2fa795d0271a71c133a8f17c19a2a625e1976ad716329e0e61789f9fd8ee0091"

# Preserve the complete package payload, including OpenSSL modules and licenses.
# Use its real control fields instead of synthesizing dependencies/installed size.
COPY --from=packages /tmp/extract/ /
COPY --from=packages /tmp/extract-status /var/lib/dpkg/status.d/libssl3t64
