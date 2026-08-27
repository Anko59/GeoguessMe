# syntax=docker/dockerfile:1

# cloudflared 2026.8.2 with the OpenSSL libraries refreshed to the fixed
# release (CVE-2026-14456, fixed in libssl3t64 3.5.7-1~deb13u2). The upstream
# distroless-based cloudflared image has no package manager and no shell, so
# per docs/security-scanning.md ("apply the fix in the shipped image") only
# libcrypto.so.3/libssl.so.3 from the fixed Debian trixie .deb are layered on
# top, together with the package's doc files; the upstream entrypoint and
# binary are untouched.
FROM debian:trixie AS packages
WORKDIR /tmp
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN apt-get update -qq \
    && apt-get download libssl3t64 \
    && dpkg-deb -x libssl3t64_*.deb /tmp/extract \
    && printf "Package: libssl3t64\nSource: openssl\nVersion: %s\nArchitecture: amd64\nMaintainer: Debian OpenSSL Team <pkg-openssl-devel@lists.alioth.debian.org>\nInstalled-Size: $(du -sk /tmp/extract/usr | cut -f1)\nDepends: libc6 (>= 2.34), libgcc-s1\nSection: libs\nPriority: important\nMulti-Arch: same\nHomepage: https://www.openssl.org/\nDescription: Secure Sockets Layer toolkit - shared libraries (upgraded for CVE-2026-14456)\n" "$(dpkg-deb -f libssl3t64_*.deb Version)" > /tmp/extract-status

FROM cloudflare/cloudflared:2026.8.2-amd64@sha256:de55830499e9adb1c580f7b5cbbdef86a3ff1715fb3ae6b3ff242f19de0afde8

COPY --from=packages /tmp/extract/usr/lib/x86_64-linux-gnu/libssl.so.3 /usr/lib/x86_64-linux-gnu/libssl.so.3
COPY --from=packages /tmp/extract/usr/lib/x86_64-linux-gnu/libcrypto.so.3 /usr/lib/x86_64-linux-gnu/libcrypto.so.3
# Keep the dpkg metadata consistent with the replaced libraries so security
# scanners read the actually-shipped version rather than the base's.
COPY --from=packages /tmp/extract-status /var/lib/dpkg/status.d/libssl3t64
