# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280
RUN GOMODCACHE=/tmp/air-mod GOCACHE=/tmp/air-cache go install github.com/air-verse/air@v1.61.7 \
    # ffmpeg/ffprobe for the in-process media-processing worker in development.
    && apk add --no-cache ffmpeg=8.1.3-r0 \
    && rm -rf /tmp/air-mod /tmp/air-cache
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
