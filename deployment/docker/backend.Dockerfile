# syntax=docker/dockerfile:1
# Production backend image. Build context is the repository root so both the
# backend and frontend images share one root .dockerignore.
# Go 1.26.6-alpine (immutable index digest).
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags='-s -w' -o /out/geoguessme .

# Alpine 3.24 runtime with ffmpeg 8.1.2 (immutable index digest). The in-process
# media-processing worker (MEDIA_PROCESSING_WORKER) validates and transcodes
# quarantined video uploads with ffprobe/ffmpeg and re-execs this binary as an
# rlimit trampoline, so the runtime image must ship a shell plus ffmpeg and
# ffprobe; a distroless base cannot provide them. ffmpeg 8.1.2-r0 is pinned for
# reproducibility and is the version the F-01 audit-images gate scans. The
# non-root UID 65532 matches the previous distroless nonroot user.
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
LABEL org.opencontainers.image.base.name="alpine:3.24" \
    org.opencontainers.image.base.digest="sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b"
RUN apk add --no-cache ffmpeg=8.1.2-r0 \
    && addgroup -S -g 65532 appuser \
    && adduser -S -u 65532 -G appuser appuser
COPY --from=build /out/geoguessme /usr/local/bin/geoguessme
EXPOSE 8080
USER appuser:appuser
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/usr/local/bin/geoguessme", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/geoguessme"]
