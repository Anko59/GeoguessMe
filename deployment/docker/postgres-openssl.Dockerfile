# syntax=docker/dockerfile:1

# PostgreSQL 15.19 runtime with the OpenSSL and libuuid packages refreshed to
# fixed releases (CVE-2026-14456 and CVE-2026-53612/CVE-2026-53613/
# CVE-2026-53614/CVE-2026-76642/CVE-2026-78408/CVE-2026-78409/
# CVE-2026-78410). The upstream postgres:15-alpine image still ships the
# vulnerable package versions, and per docs/security-scanning.md ("apply the
# fix in the shipped image when the dependency can be safely rebuilt") the
# database image upgrades only those packages from the same pinned alpine
# repository; nothing else in the upstream image (entrypoint, gosu, postgres
# binaries) is modified.
FROM postgres:15-alpine@sha256:a2c20749c564b4eb73a77bfda626f8a3cde1bbfae020fb97c616a00cdc1a2181

RUN apk add --no-cache \
    'openssl>=3.5.8-r0' \
    'libuuid>=2.42.3-r1'

LABEL org.opencontainers.image.base.name="postgres:15-alpine" \
    org.opencontainers.image.base.digest="sha256:a2c20749c564b4eb73a77bfda626f8a3cde1bbfae020fb97c616a00cdc1a2181"
