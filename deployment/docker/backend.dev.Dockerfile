# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.25.13-alpine@sha256:844b27705f54e73773e0f9bc3c780633b9d7f4b4831bf35cdad02a81a4c80bd0
RUN go install github.com/air-verse/air@v1.61.7 \
    # ffmpeg/ffprobe for the in-process media-processing worker in development.
    && apk add --no-cache ffmpeg=8.1.2-r0
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
