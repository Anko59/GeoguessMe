# syntax=docker/dockerfile:1
# Development frontend image running Vite with HMR. Build context: repo root.
FROM node:22.23.3-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
COPY frontend/vendor/braces-security/ ./vendor/braces-security/
RUN npm install
COPY frontend/ ./
CMD ["npm", "run", "dev", "--", "--host"]
