# syntax=docker/dockerfile:1
# Development frontend image running Vite with HMR. Build context: repo root.
FROM node:22.23.4-alpine@sha256:fe4f6d74e20370acb04ad889077d14d0f64a9c8ea6a42bc60cddf1e844699914
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
COPY frontend/vendor/braces-security/ ./vendor/braces-security/
RUN npm install
COPY frontend/ ./
CMD ["npm", "run", "dev", "--", "--host"]
