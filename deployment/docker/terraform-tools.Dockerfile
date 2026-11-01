# syntax=docker/dockerfile:1
# Terraform 1.16.5 already includes the fixed Go dependency graph. Preserve
# its official binary, entrypoint and CLI; refresh only the Alpine PCRE2 package
# for CVE-2026-103111 rather than recompiling Terraform from source.
FROM hashicorp/terraform:1.17.1@sha256:a5d5a5a892c98f60fcbfa81eac2a40fc19e7b3fa7719bccae10ff6111d22524a
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN apk add --no-cache --upgrade 'pcre2=10.49-r0' \
    && apk info -v | grep -Fxq 'pcre2-10.49-r0'

ARG DEPENDENCY_INPUTS
LABEL dev.geoguessme.dependency-inputs="${DEPENDENCY_INPUTS}" \
    org.opencontainers.image.base.name="hashicorp/terraform:1.16.5" \
    org.opencontainers.image.base.digest="sha256:c7926feace05d0f7e73542842bf3945924e955a1f782cf000ccbb8d18fa42d77" \
    org.opencontainers.image.version="1.16.5-pcre2-10.49-r0"
