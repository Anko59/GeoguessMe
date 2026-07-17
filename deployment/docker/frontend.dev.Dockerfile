# syntax=docker/dockerfile:1
# Development frontend image running Vite with HMR. Build context: repo root.
FROM node:22-alpine
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install
COPY frontend/ ./
CMD ["npm", "run", "dev", "--", "--host"]
