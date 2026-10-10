import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import vm from 'node:vm';
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
const requireFrontend = createRequire('/workspace/frontend/package.json');
const requireMicromatch = createRequire(requireFrontend.resolve('micromatch'));
// Verify the consumer uses the managed root dependency, not a stale nested copy.
assert.equal(requireMicromatch.resolve('braces'), requireFrontend.resolve('braces'));
const installedRoot = path.dirname(requireMicromatch.resolve('braces/package.json'));
const reviewedRoot = '/workspace/frontend/vendor/braces-security';
const attestedFiles = [
    'index.js',
    'lib/compile.js',
    'lib/constants.js',
    'lib/expand.js',
    'lib/parse.js',
    'lib/stringify.js',
    'lib/utils.js',
    'LICENSE',
];
const provenance = JSON.parse(fs.readFileSync(path.join(reviewedRoot, 'provenance.json'), 'utf8'));
const digest = (file) => createHash('sha256').update(fs.readFileSync(file)).digest('hex');
for (const file of attestedFiles) {
    assert.equal(
        digest(path.join(reviewedRoot, file)),
        provenance.patchedSha256[file],
        `Reviewed ${file} differs from its provenance checksum`,
    );
    assert.equal(
        digest(path.join(installedRoot, file)),
        provenance.patchedSha256[file],
        `Installed ${file} differs from reviewed backport`,
    );
}
const installedMetadata = JSON.parse(fs.readFileSync(path.join(installedRoot, 'package.json'), 'utf8'));
assert.equal(installedMetadata.name, '@geoguessme/braces-security');
assert.equal(installedMetadata.private, true);
const braces = (await import('braces')).default;
const micromatch = requireFrontend('micromatch');

// Exercise the installed dependency, not a stand-in that can hide a bad lockfile.
// GHSA-vfj7-8cjw-p6xm affects parsing and every recursive public AST walker.
const methods = ['parse', 'compile', 'expand', 'stringify'];
const nested = (opening, closing, depth) => opening.repeat(depth) + 'a' + closing.repeat(depth);
const astAtDepth = (depth) => {
    let node = { type: 'text', value: 'a' };
    for (let i = 0; i < depth; i++) node = { type: 'brace', nodes: [node] };
    return { type: 'root', nodes: [node] };
};

for (const method of methods) {
    for (const [opening, closing] of [
        ['{', '}'],
        ['(', ')'],
    ]) {
        test(`${method} accepts depth 100 and rejects depth 101 (${opening})`, () => {
            assert.doesNotThrow(() => braces[method](nested(opening, closing, 100)));
            assert.throws(() => braces[method](nested(opening, closing, 101)), /exceeds max depth/);
        });
    }
    test(`${method} cannot disable the hard cap with an oversized or nonfinite option`, () => {
        for (const maxDepth of [10000, Infinity, -Infinity, NaN, '10000', null]) {
            assert.throws(() => braces[method](nested('{', '}', 101), { maxDepth }), /exceeds max depth/);
        }
    });
    test(`${method} bounds mixed and malformed nesting without counting literal contexts`, () => {
        assert.doesNotThrow(() => braces[method]('{('.repeat(50) + 'a' + ')}'.repeat(50)));
        for (const pattern of ['{('.repeat(51) + 'a' + ')}'.repeat(51), '{)'.repeat(101), '(}'.repeat(101)]) {
            assert.throws(() => braces[method](pattern), /exceeds max depth/);
        }
        for (const pattern of ['\\{a\\}', '[{a}]', '"{a}"', "'{a}'"]) {
            assert.doesNotThrow(() => braces[method](pattern, { maxDepth: 0 }));
        }
        assert.doesNotThrow(() => braces[method]('{a}'.repeat(200), { maxDepth: 1 }));
    });
    test(`${method} enforces stricter fractional depth limits`, () => {
        assert.doesNotThrow(() => braces[method]('{a}', { maxDepth: 1.5 }));
        assert.throws(() => braces[method]('{{a}}', { maxDepth: 1.5 }), /exceeds max depth/);
        assert.doesNotThrow(() => braces[method]('(a)', { maxDepth: 1.5 }));
        assert.throws(() => braces[method]('((a))', { maxDepth: 1.5 }), /exceeds max depth/);
    });
}

for (const method of ['compile', 'expand', 'stringify']) {
    test(`${method} bounds caller-supplied ASTs without relying on parse`, () => {
        assert.throws(() => braces[method](astAtDepth(101)), /exceeds max depth/);
        assert.throws(() => braces[method](astAtDepth(4000)), /exceeds max depth/);
        assert.throws(() => braces[method](astAtDepth(2), { maxDepth: 1.5 }), /exceeds max depth/);
        for (const maxDepth of [10000, Infinity, NaN]) {
            assert.throws(() => braces[method](astAtDepth(101), { maxDepth }), /exceeds max depth/);
        }
        for (const flag of ['invalid', 'dollar']) {
            const ast = astAtDepth(101);
            ast.nodes[0][flag] = true;
            assert.throws(() => braces[method](ast), /exceeds max depth/);
        }
        const root = { type: 'root', nodes: [] };
        root.nodes.push(root);
        assert.throws(() => braces[method](root), /exceeds max depth/);
        const first = { type: 'brace', nodes: [] };
        const second = { type: 'brace', nodes: [first] };
        first.nodes.push(second);
        assert.throws(() => braces[method]({ type: 'root', nodes: [first] }), /exceeds max depth/);
    });
}

for (const cycle of ['self', 'multiple']) {
    test(`expand rejects a ${cycle} parent cycle instead of hanging`, () => {
        const ast = { type: 'paren', nodes: [{ type: 'text', value: 'a' }] };
        if (cycle === 'self') ast.parent = ast;
        else {
            const parent = { type: 'paren', parent: ast };
            ast.parent = parent;
        }
        // An unsafe upgrade must fail deterministically, not hang the entire gate.
        assert.throws(
            () => vm.runInNewContext('expand(ast)', { expand: braces.expand, ast }, { timeout: 250 }),
            /parent chain contains a cycle/,
        );
    });
}

test('ordinary ranges, alternatives, parentheses and escaping remain compatible', () => {
    assert.deepEqual(braces.expand('src/{a,b}-{1..2}.css'), [
        'src/a-1.css',
        'src/a-2.css',
        'src/b-1.css',
        'src/b-2.css',
    ]);
    assert.deepEqual(braces.expand('foo/({a,b})'), ['foo/(a)', 'foo/(b)']);
    // Keep release3.0.3 semantics: no unrelated upstream-master quote changes.
    assert.deepEqual(braces("foo'{a,b}"), ['foo{a,b}']);
    assert.deepEqual(braces.expand("foo'{a,b}"), ['foo{a,b}']);
    assert.deepEqual(braces("foo'(bar"), ['foo(bar']);
    for (const pattern of ['{{a}}', '{a,{b}}', '{{x}y}', '{a,{b,{c}}', '{}{a}']) {
        assert.equal(braces.stringify(braces.parse(pattern), { escapeInvalid: true }), pattern);
    }
    assert.deepEqual(micromatch(['src/a.css', 'src/b.scss', 'src/c.ts'], 'src/*.{css,scss}'), [
        'src/a.css',
        'src/b.scss',
    ]);
});
