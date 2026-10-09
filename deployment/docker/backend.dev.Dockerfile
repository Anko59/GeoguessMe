# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0
RUN GOMODCACHE=/tmp/air-mod GOCACHE=/tmp/air-cache go install github.com/air-verse/air@v1.61.7 \
    # ffmpeg/ffprobe for the in-process media-processing worker in development.
    && apk add --no-cache ffmpeg=8.1.2-r0 \
    && rm -rf /tmp/air-mod /tmp/air-cache
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
