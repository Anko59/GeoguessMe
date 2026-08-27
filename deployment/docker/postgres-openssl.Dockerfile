# syntax=docker/dockerfile:1

# PostgreSQL 15.19 runtime with the OpenSSL packages refreshed to at least the
# fixed release (CVE-2026-14456, fixed in 3.5.8-r0). Every published
# postgres:15-alpine image so far still ships libcrypto3/libssl3 3.5.7-r0, and
# per docs/security-scanning.md ("apply the fix in the shipped image when the
# dependency can be safely rebuilt") the database image therefore upgrades only
# those two packages from the same pinned alpine repository; nothing else in
# the upstream image (entrypoint, gosu, postgres binaries) is modified.
FROM postgres:15-alpine@sha256:a2c20749c564b4eb73a77bfda626f8a3cde1bbfae020fb97c616a00cdc1a2181

RUN apk add --no-cache 'openssl>=3.5.8-r0'

LABEL org.opencontainers.image.base.name="postgres:15-alpine" \
    org.opencontainers.image.base.digest="sha256:a2c20749c564b4eb73a77bfda626f8a3cde1bbfae020fb97c616a00cdc1a2181"
