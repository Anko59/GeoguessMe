# syntax=docker/dockerfile:1
# Validation-only Caddy image matching the patched Caddy binary used by the
# production frontend image.
FROM caddy:2.11.4-builder-alpine@sha256:8e89605351333ad2cc2f3bcc95275a2ccc427f88914050e86a5fde0fd77a63c4 AS caddy-build
RUN xcaddy build v2.11.4 \
    --output /usr/bin/caddy \
    --replace 'golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0'

FROM caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648
COPY --from=caddy-build /usr/bin/caddy /usr/bin/caddy
RUN setcap cap_net_bind_service=+ep /usr/bin/caddy
