# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb
RUN GOMODCACHE=/tmp/air-mod GOCACHE=/tmp/air-cache go install github.com/air-verse/air@v1.61.7 \
    # ffmpeg/ffprobe for the in-process media-processing worker in development.
    && apk add --no-cache ffmpeg=8.1.3-r0 \
    && rm -rf /tmp/air-mod /tmp/air-cache
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
