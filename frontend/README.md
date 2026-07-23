# GeoGuessMe frontend

The frontend is a React and TypeScript application built with Vite. Use the
repository-level Dockerized Make targets for all development, testing, linting,
formatting, and builds; do not run Node or package-manager commands directly on
the host.

## Structure

- `src/pages` contains route-level experiences.
- `src/components` contains gameplay, navigation, and reusable UI components.
- `src/faceFilters.ts` contains the local Canvas2D overlays used with Jeeliz
  face tracking in the camera composer.
- `src/styles` contains the global design tokens, base rules, primitives, and
  motion policy.
- `public/Identity.md` defines the brand and protected visualization colors.
- `e2e` contains Playwright interaction, accessibility, and viewport coverage.

Page-specific CSS should consume the shared tokens instead of introducing a new
palette. Keep branded gradients focused on primary actions and meaningful
states. Leaderboard and map color semantics are compatibility-sensitive and must
follow `public/Identity.md`.

## Canonical commands

From the repository root:

```text
make dev
make test-frontend
make lint-frontend
make lint-css
make type-check
make build-frontend
make verify
```

The development application is exposed through the Docker Compose frontend
service documented in the root README and local-development guide.
