// Security and provenance contracts for the explicit local CVE-2026-93687 backport.
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { createRequire } = require("node:module");
const { spawnSync } = require("node:child_process");
const { test } = require("node:test");

const root = path.resolve(__dirname, "../../..");
const vendor = path.join(root, "frontend/vendor/geoguessme-patched-braces");
const frontendRequire = createRequire(path.join(root, "frontend/package.json"));
const micromatchRequire = createRequire(frontendRequire.resolve("micromatch"));
const braces = micromatchRequire("braces");
const provenance = JSON.parse(fs.readFileSync(path.join(vendor, "provenance.json"), "utf8"));
const sourceFiles = [
  "LICENSE", "index.js", "lib/compile.js", "lib/constants.js", "lib/expand.js",
  "lib/parse.js", "lib/stringify.js", "lib/utils.js",
];
const sha256 = (file) => createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const depthError = (error) => error instanceof RangeError && /exceeds max depth/.test(error.message);

function directory(t) {
  const value = fs.mkdtempSync(path.join(os.tmpdir(), "braces-backport-"));
  t.after(() => fs.rmSync(value, { recursive: true, force: true }));
  return value;
}

function applyPatch(cwd, reverse = false) {
  const args = ["apply", "--unidiff-zero", ...(reverse ? ["--reverse"] : []), path.join(vendor, "security.patch")];
  const result = spawnSync("git", args, { cwd, encoding: "utf8", timeout: 15000 });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
}

function originalSource(t) {
  const target = directory(t);
  for (const file of sourceFiles) {
    fs.mkdirSync(path.dirname(path.join(target, file)), { recursive: true });
    fs.copyFileSync(path.join(vendor, file), path.join(target, file));
  }
  applyPatch(target, true);
  for (const file of sourceFiles) {
    assert.equal(sha256(path.join(target, file)), provenance.files[file].upstreamSha256, file);
  }
  // The reconstructed upstream implementation uses the same audited fill-range dependency.
  fs.symlinkSync(path.join(root, "frontend/node_modules"), path.join(target, "node_modules"));
  return target;
}

function nestedAst(depth) {
  let node = { type: "text", value: "x" };
  for (let i = 0; i < depth; i++) node = { type: "paren", nodes: [node] };
  return { type: "root", nodes: [node] };
}

function nestedText(depth, open = "{", close = "}") {
  return open.repeat(depth) + "x" + close.repeat(depth);
}

test("consumer resolution uses the honest local package and the reviewed bytes", () => {
  assert.equal(fs.realpathSync(micromatchRequire.resolve("braces")), path.join(vendor, "index.js"));
  assert.equal(micromatchRequire("braces/package.json").name, "geoguessme-patched-braces");
  assert.equal(micromatchRequire("braces/package.json").version, "1.0.0");
  assert.equal(micromatchRequire("braces/package.json").private, true);
  assert.equal(provenance.schemaVersion, 1);
  assert.equal(provenance.upstream.package, "braces@3.0.3");
  assert.equal(provenance.upstream.gitCommit, "74b2db2938fad48a2ea54a9c8bf27a37a62c350d");
  assert.equal(provenance.upstream.integrity,
    "sha512-yQbXgO/OSZVD2IsiLlro+7Hf6Q18EJrKSEsdoMzKePKXct3gvD8oLcOQdIzGupr5Fj+EDe8gO/lxc1BzfMpxvA==");
  assert.equal(provenance.security.advisory, "GHSA-vfj7-8cjw-p6xm");
  assert.equal(provenance.security.patchReferenceCommit, "d0d575e55e74a4e0218e5248fafb79efc3e54ebb");
  assert.equal(provenance.security.maximumDepth, 100);
  assert.deepEqual(Object.keys(provenance.files).sort(), sourceFiles);
  assert.deepEqual(fs.readdirSync(path.join(vendor, "lib")).sort(),
    sourceFiles.filter((file) => file.startsWith("lib/")).map((file) => path.basename(file)));
  for (const file of sourceFiles) {
    assert.equal(sha256(path.join(vendor, file)), provenance.files[file].patchedSha256, file);
  }
  assert.equal(sha256(path.join(vendor, "security.patch")), provenance.security.localPatchSha256);
});

test("security.patch reverses to pinned upstream bytes and reproduces the backport", (t) => {
  const original = originalSource(t);
  applyPatch(original);
  for (const file of sourceFiles) {
    assert.equal(sha256(path.join(original, file)), provenance.files[file].patchedSha256, file);
  }
});

for (const method of ["parse", "compile", "expand", "stringify"]) {
  test(`${method} rejects deeply nested brace, parenthesis and mixed text`, () => {
    for (const depth of [101, 4000]) {
      for (const [open, close] of [["{", "}"], ["(", ")"]]) {
        assert.throws(() => braces[method](nestedText(depth, open, close)), depthError);
      }
    }
    assert.throws(() => braces[method](nestedText(51, "{(", ")}")), depthError);
    assert.throws(() => braces[method]("{".repeat(101)), depthError);
  });

  test(`${method} permits depth 100, nonnested patterns and quoted/escaped literals`, () => {
    for (const [open, close] of [["{", "}"], ["(", ")"]]) {
      assert.doesNotThrow(() => braces[method](nestedText(100, open, close)));
    }
    assert.doesNotThrow(() => braces[method](nestedText(50, "{(", ")}")));
    assert.doesNotThrow(() => braces[method]("{x}".repeat(500)));
    assert.doesNotThrow(() => braces[method]('"' + "{".repeat(200) + '"'));
    assert.doesNotThrow(() => braces[method]("\\{".repeat(200)));
  });

  test(`${method} caps attempted maxDepth bypasses and supports stricter limits`, () => {
    for (const maxDepth of [1000, Infinity, -Infinity, NaN, null, "1000", {}, -1]) {
      assert.throws(() => braces[method](nestedText(101), { maxDepth }), depthError);
    }
    assert.doesNotThrow(() => braces[method](nestedText(2), { maxDepth: 2 }));
    assert.throws(() => braces[method](nestedText(3), { maxDepth: 2 }), depthError);
    assert.throws(() => braces[method]("{x}", { maxDepth: 0 }), depthError);
    let reads = 0;
    const accessor = { get maxDepth() { return ++reads === 1 ? 1 : NaN; } };
    assert.throws(() => braces[method](nestedText(101), accessor), depthError);
    assert.equal(reads, 1, "maxDepth must be captured once before finite/clamp checks");
  });
}

for (const method of ["compile", "expand", "stringify"]) {
  test(`${method} rejects prebuilt deep/cyclic ASTs, not only parser-generated input`, () => {
    assert.doesNotThrow(() => braces[method](nestedAst(100)));
    for (const depth of [101, 4000]) {
      assert.throws(() => braces[method](nestedAst(depth)), depthError);
      assert.throws(() => braces[method](nestedAst(depth).nodes[0]), depthError);
    }
    const cycle = { type: "root", nodes: [] };
    cycle.nodes.push(cycle);
    assert.throws(() => braces[method](cycle), depthError);
    for (const maxDepth of [1000, Infinity, -Infinity, NaN, null, "1000", {}, -1]) {
      assert.throws(() => braces[method](nestedAst(101), { maxDepth }), depthError);
    }
    assert.throws(() => braces[method](nestedAst(3), { maxDepth: 2 }), depthError);
    let reads = 0;
    const accessor = { get maxDepth() { return ++reads === 1 ? 1 : NaN; } };
    assert.throws(() => braces[method](nestedAst(101), accessor), depthError);
    assert.equal(reads, 1, "direct AST entry captures maxDepth once");
  });
}

test("expand bounds cyclic and excessive parent-pointer traversal", () => {
  // A subprocess deadline makes a reintroduced infinite loop a deterministic failure.
  const result = spawnSync(process.execPath, ["-e", `
    const assert = require('node:assert/strict');
    const braces = require(${JSON.stringify(path.join(vendor, "index.js"))});
    const rejects = node => assert.throws(() => braces.expand(node),
      error => error instanceof RangeError && /exceeds max depth/.test(error.message));
    const self = { type: 'paren', nodes: [] }; self.parent = self; rejects(self);
    const a = { type: 'paren', nodes: [] }, b = { type: 'paren', nodes: [] };
    a.parent = b; b.parent = a; rejects(a);
    const long = { type: 'paren', nodes: [] }; let parent = long;
    for (let i = 0; i < 101; i++) parent = parent.parent = { type: 'paren', nodes: [] };
    rejects(long);
  `], { encoding: "utf8", timeout: 5000 });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
});

test("expand fallback stringify preserves the remaining absolute depth budget", () => {
  for (const fallback of ["invalid", "dollar", "range"]) {
    const ast = nestedAst(4);
    const inner = ast.nodes[0].nodes[0];
    if (fallback === "range") inner.ranges = 1;
    else inner[fallback] = true;
    assert.throws(() => braces.expand(ast, { maxDepth: 3 }), depthError, fallback);
    let reads = 0;
    const accessor = { get maxDepth() { return ++reads === 1 ? 3 : NaN; } };
    assert.throws(() => braces.expand(ast, accessor), depthError, fallback);
    assert.equal(reads, 1, "fallback must reuse the captured limit");
    assert.doesNotThrow(() => braces.expand(ast, { maxDepth: 4 }), fallback);
  }
});

for (const method of ["compile", "expand", "stringify"]) {
  test(`${method} rejects nonstring AST values before append, flatten or coercion`, () => {
    const cycle = []; cycle.push(cycle);
    let deep = ["x"];
    for (let i = 0; i < 4000; i++) deep = [deep];
    for (const value of [cycle, deep, [], {}, 1, null]) {
      const ast = { type: "root", nodes: [{ type: "text", value }] };
      assert.throws(() => braces[method](ast), { name: "TypeError", message: "Expected AST value to be a string" });
    }
  });
}

test("all ordinary public methods preserve the original 3.0.3 results", (t) => {
  const original = require(path.join(originalSource(t), "index.js"));
  const patterns = [
    "", "plain.css", "src/**/*.{ts,tsx}", "a/{b,c}/d", "x{1..3}y", "{01..03}",
    "{a..e..2}", "foo/{a,{b,c}}/bar", "{a,b}{1,2}", "${literal}", "{x}",
    "a{b", "a}b", "{,a,a}", "\\{a,b\\}", "[{}]", "(a{b,c})", '"{a,b}"',
  ];
  for (const options of [{}, { expand: true }, { nodupes: true, noempty: true },
    { escapeInvalid: true }, { keepEscaping: true, keepQuotes: true }]) {
    for (const pattern of patterns) {
      assert.deepEqual(braces(pattern, options), original(pattern, options), pattern);
      for (const method of ["compile", "expand", "stringify"]) {
        assert.deepEqual(braces[method](pattern, options), original[method](pattern, options), `${method}: ${pattern}`);
      }
      assert.equal(braces.stringify(braces.parse(pattern, options), options),
        original.stringify(original.parse(pattern, options), options), pattern);
    }
  }
});

test("actual micromatch, fast-glob, globby and stylelint consumers retain glob/brace behavior", async (t) => {
  // Resolve each consumer's own chain, not merely the frontend-hoisted copy.
  for (const consumer of ["fast-glob", "globby", "stylelint"]) {
    const consumerRequire = createRequire(frontendRequire.resolve(consumer));
    const globRequire = consumer === "fast-glob" ? consumerRequire
      : createRequire(consumerRequire.resolve("fast-glob"));
    const matcherRequire = createRequire(globRequire.resolve("micromatch"));
    assert.equal(fs.realpathSync(matcherRequire.resolve("braces")), path.join(vendor, "index.js"), consumer);
    assert.throws(() => matcherRequire("braces").expand(nestedText(101)), depthError, consumer);
  }
  const micromatch = frontendRequire("micromatch");
  assert.deepEqual(micromatch.braces("src/*.{ts,tsx}"), ["src/*.(ts|tsx)"]);
  assert.deepEqual(micromatch.braceExpand("src/*.{ts,tsx}"), ["src/*.ts", "src/*.tsx"]);
  assert.throws(() => micromatch.braces(nestedText(4000)), depthError);
  const target = directory(t);
  fs.mkdirSync(path.join(target, "src"));
  for (const file of ["a.css", "b.scss", "c.js"]) fs.writeFileSync(path.join(target, "src", file), "");
  const fastGlob = frontendRequire("fast-glob");
  assert.deepEqual(fastGlob.sync("src/*.{css,scss}", { cwd: target }).sort(), ["src/a.css", "src/b.scss"]);
  const { globby } = await import(frontendRequire.resolve("globby"));
  assert.deepEqual((await globby(["src/*.{css,scss}", "!src/b.scss"], { cwd: target })).sort(), ["src/a.css"]);
  fs.writeFileSync(path.join(target, "src/a.css"), ".a { color: #000; }\n");
  fs.writeFileSync(path.join(target, "src/b.scss"), ".b { color: #xyz; }\n");
  const { default: stylelint } = await import(frontendRequire.resolve("stylelint"));
  const result = await stylelint.lint({
    files: "src/*.{css,scss}",
    cwd: target,
    config: { rules: { "color-no-invalid-hex": true } },
  });
  assert.equal(result.errored, true);
  assert.deepEqual(result.results.map((file) => path.relative(target, file.source)).sort(), ["src/a.css", "src/b.scss"]);
  assert.equal(result.results.find((file) => file.source.endsWith("a.css")).warnings.length, 0);
  assert.equal(result.results.find((file) => file.source.endsWith("b.scss")).warnings[0].rule, "color-no-invalid-hex");
});
