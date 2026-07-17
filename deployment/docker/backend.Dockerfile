# syntax=docker/dockerfile:1
# Production backend image. Build context is the repository root so both the
# backend and frontend images share one root .dockerignore.
# Go 1.25.12-alpine (immutable index digest).
FROM golang:1.25.12-alpine@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587 AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags='-s -w' -o /out/geoguessme .

# Distroless static Debian 12 nonroot (immutable index digest).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b
COPY --from=build /out/geoguessme /usr/local/bin/geoguessme
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/usr/local/bin/geoguessme", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/geoguessme"]
