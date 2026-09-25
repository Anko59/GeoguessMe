FROM quay.io/keycloak/keycloak:26.7.4@sha256:82a77884f3af238beab1e7afd63b5f530e1b5c0590bd7aa60b40a40463e29b2c

# Keep the upstream distribution and replace only its vulnerable FreeMarker
# runtime with the exact 2.3.35 artifact Keycloak has already selected on its
# release/26.7 maintenance branch. Preserve the original class-path filename.
USER 0
ADD https://repo.maven.apache.org/maven2/org/freemarker/freemarker/2.3.35/freemarker-2.3.35.jar \
    /opt/keycloak/lib/lib/main/org.freemarker.freemarker-2.3.32.jar
RUN printf '%s  %s\n' \
        0fac87dddd78f1223139e8ef88e819c7f483c0a3835cdf5982ad5e4576d1d896 \
        /opt/keycloak/lib/lib/main/org.freemarker.freemarker-2.3.32.jar \
        >/tmp/freemarker.sha256 \
    && sha256sum -c /tmp/freemarker.sha256 \
    && chmod 0644 /opt/keycloak/lib/lib/main/org.freemarker.freemarker-2.3.32.jar \
    && rm -f /tmp/freemarker.sha256

LABEL org.opencontainers.image.base.name="quay.io/keycloak/keycloak:26.7.4" \
      org.opencontainers.image.base.digest="sha256:82a77884f3af238beab1e7afd63b5f530e1b5c0590bd7aa60b40a40463e29b2c" \
      org.opencontainers.image.description="Keycloak 26.7.4 with FreeMarker 2.3.35"

ARG GEOGUESSME_REVISION
LABEL org.opencontainers.image.revision="${GEOGUESSME_REVISION}"

USER 1000
