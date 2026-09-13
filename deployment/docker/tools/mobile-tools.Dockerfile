# syntax=docker/dockerfile:1
FROM eclipse-temurin:21-jdk-jammy@sha256:ce5767b7222312d42395f5bab033cd91f09e44032a2f21bdfd7b5b912dbe1e77

ARG MAESTRO_VERSION=cli-2.10.0
ARG MAESTRO_SHA256=29b675e10cc12080e445e9bfb2e2b4e4dfb9c0f2e30d5884120d258b5e1cd991

# Keep the runtime immutable while allowing this disposable tool image to take
# current Jammy security revisions instead of pinning obsolete apt builds.
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        ca-certificates curl jq unzip \
        libasound2 libdbus-1-3 libdrm2 libfontconfig1 libgl1 libnss3 libpulse0 \
        libx11-6 libx11-xcb1 libxcb1 libxcomposite1 libxcursor1 libxdamage1 \
        libxi6 libxkbcommon0 libxrandr2 libxtst6 \
    && rm -rf /var/lib/apt/lists/* \
    && curl --fail --location --silent --show-error \
        "https://github.com/mobile-dev-inc/Maestro/releases/download/${MAESTRO_VERSION}/maestro.zip" \
        --output /tmp/maestro.zip \
    && echo "${MAESTRO_SHA256}  /tmp/maestro.zip" | sha256sum --check --strict \
    && unzip -q /tmp/maestro.zip -d /opt \
    && ln -s /opt/maestro/bin/maestro /usr/local/bin/maestro \
    && rm /tmp/maestro.zip

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    PATH=/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools:/opt/android-sdk/emulator:/opt/maestro/bin:$PATH

WORKDIR /workspace
