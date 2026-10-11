# Maintained braces security backport

This private, MIT-licensed downstream package is **not an upstream release**. It
retains the published `braces@3.0.3` implementation and dependencies, with only
the nesting-depth and cyclic-AST guards from
[upstream PR 72](https://github.com/micromatch/braces/pull/72), reviewed at
commit `28d440b5dd449dbf1fe6f3506cf94ecca4d02660`.

## Provenance and scope

The original npm artifact is
[braces-3.0.3.tgz](https://registry.npmjs.org/braces/-/braces-3.0.3.tgz). Its
SHA-512 integrity was verified before applying the patch.
[provenance.json](provenance.json) records that integrity, the exact upstream
base/head commits, the immutable comparison diff SHA-256, and SHA-256 checksums
for all original and patched runtime files plus the retained
[MIT license](LICENSE).

Only `lib/*` hunks from the pinned upstream comparison were applied. The public
entrypoint, utility implementation, license, and `fill-range` dependency remain
unchanged. Four modified library files match the reviewed upstream head byte for
byte. The parser intentionally retains release 3.0.3 quote/comma behavior;
copying the entire upstream-head parser would import unrelated unreleased fixes.
Normal and malformed quote behavior was compared against the published release
before the backport was installed.

The code changes address
[GHSA-vfj7-8cjw-p6xm / CVE-2026-93687](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm):

- Parsing bounds combined brace/parenthesis nesting at 100 levels.
- Compile, expand, and stringify independently guard caller-supplied AST depth.
- Larger `maxDepth` options cannot disable the cap; stricter fractional limits
  work.
- Expansion rejects cyclic parent chains rather than looping indefinitely.
- Stringify preserves valid nested-brace behavior with `escapeInvalid: true`.

The existing AST contract still expects string text values. These targeted
guards are not a general schema validator for arbitrary JavaScript objects or a
cap on all expansion output volume.

## Installation and maintenance

The frontend declares this maintained security dependency directly and overrides
transitive `braces` imports with the same local specification. The explicit root
declaration anchors npm's path resolution when upgrading the old registry lock.
`install-links=true` is required: npm's default local override link can point
inside micromatch rather than this directory. The scoped name and
`3.0.3-geoguessme.1` version identify a genuinely patched local derivative; they
do not claim a new official braces release. Merely renaming vulnerable source,
changing audit thresholds, or ignoring the advisory is not acceptable.

Version-based npm audit still flags the patched upstream Git tarball because its
manifest remains `braces@3.0.3`. The mandatory security regressions therefore
validate the actually installed downstream implementation as well as running the
unchanged registry audit. Preserve audited runtime bytes rather than
reformatting them; checksum comparisons detect accidental source drift.

For future changes, review the original artifact and pinned upstream diff,
refresh the checksums and downstream version, regenerate the dependency lock
with `make deps-npm-lock`, and run `make test-braces-security`, `make audit`,
`make lint-css`, and `make preflight`. Remove this override only after an
official compatible fixed release passes those same installed-code and consumer
tests.
