# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc
RUN GOMODCACHE=/tmp/air-mod GOCACHE=/tmp/air-cache go install github.com/air-verse/air@v1.61.7 \
    # ffmpeg/ffprobe for the in-process media-processing worker in development.
    && apk add --no-cache ffmpeg=8.1.2-r0 \
    && rm -rf /tmp/air-mod /tmp/air-cache
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
