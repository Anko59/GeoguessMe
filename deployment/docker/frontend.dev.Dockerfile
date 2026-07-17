# syntax=docker/dockerfile:1
# Development frontend image running Vite with HMR. Build context: repo root.
FROM node:22-alpine@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install
COPY frontend/ ./
CMD ["npm", "run", "dev", "--", "--host"]
