# syntax=docker/dockerfile:1
# Production backend image. Build context is the repository root so both the
# backend and frontend images share one root .dockerignore.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend/ ./backend/
RUN cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags='-s -w' -o /out/geoguessme .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/geoguessme /usr/local/bin/geoguessme
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/usr/local/bin/geoguessme", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/geoguessme"]
