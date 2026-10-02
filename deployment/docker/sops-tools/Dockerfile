# syntax=docker/dockerfile:1
# The current upstream SOPS release still ships Debian's vulnerable libexpat.
# Keep the official, digest-pinned SOPS binary and refresh only that package.
FROM ghcr.io/getsops/sops:v3.13.3@sha256:857f5a151ac0b2bfc55c1e4e5581d66fb8e268e4d106b38e74191f3bac9d58ea

# Keep the upstream root user: the host mounts its mode-0600 age identity read-only.
# Pin the known-fixed Debian package version and fail closed if it is no longer
# available; a later security update must be reviewed and explicitly repinned.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install --yes --no-install-recommends --only-upgrade "libexpat1=2.5.0-1+deb12u4" \
    && installed_version="$(dpkg-query -W -f='${Version}' libexpat1)" \
    && test "$installed_version" = "2.5.0-1+deb12u4" \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

LABEL org.opencontainers.image.source="https://github.com/Anko59/GeoguessMe" \
    org.opencontainers.image.description="SOPS v3.13.3 with fixed Debian libexpat" \
    org.opencontainers.image.version="3.13.3-expat-deb12u4" \
    org.opencontainers.image.base.name="ghcr.io/getsops/sops:v3.13.3" \
    org.opencontainers.image.base.digest="sha256:857f5a151ac0b2bfc55c1e4e5581d66fb8e268e4d106b38e74191f3bac9d58ea"
