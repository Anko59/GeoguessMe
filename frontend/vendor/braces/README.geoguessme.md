# GeoGuessMe braces security backport

This private package copy preserves `braces` 3.x compatibility while applying
upstream PR [micromatch/braces#72](https://github.com/micromatch/braces/pull/72)
at the pinned source commit `28d440b5dd449dbf1fe6f3506cf94ecca4d02660`. It adds
bounded nesting checks to parsing and AST walkers to address
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), plus
the follow-up compatibility fixes included in that commit.

The local package version `3.0.4+geoguessme.1` identifies this patched private
copy; it is not an upstream npm release. The `vendor-braces-security-backport`
Make target reimports the exact commit and fails closed if its pinned branch has
moved. The dependency is covered by the focused Vitest security/compatibility
tests under `frontend/src/utils/bracesSecurity/`; `make lint-css` also exercises
Stylelint's globbing consumers.

Remove this package and its local devDependency after upstream publishes a
patched release for the issue tracked at
[braces issue #73](https://github.com/micromatch/braces/issues/73).
