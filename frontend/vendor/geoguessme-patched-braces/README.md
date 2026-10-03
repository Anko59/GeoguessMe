# GeoGuessMe braces security backport

This is **not an upstream braces release**. The private local package
`geoguessme-patched-braces@1.0.0` preserves the `braces@3.0.3` CommonJS API and
MIT license, with the CVE-2026-93687 depth guards described by
[upstream PR 72](https://github.com/micromatch/braces/pull/72).

[provenance.json](provenance.json) records the npm tarball integrity, upstream
commit, reviewed PR commit, original/patched file hashes, and
[the exact local patch](security.patch). The patch uses zero-context unified
hunks (`git apply --unidiff-zero`); both verifiers require exact before/after
file hashes, so application cannot silently accept different source bytes.
Unlike the PR's stringify change, this backport preserves v3.0.3's existing
parent/escaping behavior. No other runtime behavior changes except rejecting
excessive nesting, unbounded parent traversal, and malformed nonstring AST
values.

The parser limits combined brace/parenthesis depth to 100. Compile, expand and
stringify separately enforce that limit for prebuilt ASTs. Finite `maxDepth`
values can tighten but never increase it; nonnumeric/nonfinite values use 100.
Negative values reject nested inputs; zero disallows nesting. Existing length
and expansion-range limits remain unchanged. Expand bounds parent-pointer walks,
and fallback stringify retains the current walk depth instead of resetting it.
All AST walkers reject nonstring values before expansion/coercion.

Direct AST inputs must be structurally valid parser-shaped trees with owned
parent/queue relationships. The tested depth, child/parent-cycle and nonstring
value rejections do not imply termination for arbitrary objects, executable
accessors, or forged external parent/queue structures. Untrusted string patterns
remain the supported security boundary.

`make test-braces-security-backport` verifies exact shipped source hashes,
reverse/forward patch reproducibility, all four entry points, cyclic/deep ASTs,
option bypass attempts, valid-boundary behavior, original-version result parity,
and real Micromatch/Fast-glob/Globby/Stylelint consumer resolution and behavior.
`make verify-braces-backport-source` fetches the integrity-pinned upstream npm
artifact and reconstructs each shipped source file. Both run in `make audit`.

Npm audit does **not** recognize renamed local source as the upstream advisory
package. Zero npm findings alone therefore does not prove this patch safe;
source provenance and the explicit security regressions are mandatory. The
transitive `fill-range` dependency remains in the npm audit tree. Remove this
local fork/override when a supported, patched upstream release passes the same
contracts. See the
[compatibility ledger](../../../docs/agent-engineering.md#braces-security-backport).
