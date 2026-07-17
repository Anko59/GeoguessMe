# syntax=docker/dockerfile:1
# Development backend image with Air hot reload. Build context: repository root.
FROM golang:1.24-alpine
RUN go install github.com/air-verse/air@v1.61.7
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
CMD ["air"]
