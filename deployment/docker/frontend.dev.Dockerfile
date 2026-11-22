# syntax=docker/dockerfile:1
# Development frontend image running Vite with HMR. Build context: repo root.
FROM node:22.23.5-alpine@sha256:1314fb30ec37592d5845e5a160d522c0e680972aa534549940b8d73bde88481a
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
COPY frontend/vendor/braces-security/ ./vendor/braces-security/
RUN npm install
COPY frontend/ ./
CMD ["npm", "run", "dev", "--", "--host"]
