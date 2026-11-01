// Installed-consumer regression contracts for patched tooling dependencies (#376).
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { createRequire } = require("node:module");
const { spawnSync } = require("node:child_process");
const { test } = require("node:test");
const { pathToFileURL } = require("node:url");

const root = path.resolve(__dirname, "../../..");
const frontendRequire = createRequire(path.join(root, "frontend/package.json"));
const markdownlintEntry = frontendRequire.resolve("markdownlint-cli");
const markdownlintRequire = createRequire(markdownlintEntry);

function temporaryDirectory(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "npm-overrides-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

// Accept upstream patch releases at or above the reviewed fixed version.
function assertAtLeast(version, floor) {
  const parse = (value) => value.split(/[.-]/).slice(0, 3).map(Number);
  const [actual, minimum] = [parse(version), parse(floor)];
  const order = actual.findIndex((part, index) => part !== minimum[index]);
  assert.ok(order === -1 || actual[order] > minimum[order], `${version} must be >= ${floor}`);
}

test("manifest keeps only the overrides upstream has not yet released", () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(root, "frontend/package.json"), "utf8"));
  assert.equal(manifest.devDependencies.braces, "file:vendor/braces-security");
  assert.deepEqual(manifest.overrides, {
    braces: "file:vendor/braces-security",
    "markdownlint-cli": { "js-yaml": "5.4.2", "smol-toml": "1.9.0" },
    "micromark-extension-math": { katex: "0.18.2" },
  });
});

test("Capacitor CLI resolves an xcode release without the uuid dependency", () => {
  const capacitorRequire = createRequire(frontendRequire.resolve("@capacitor/cli/package.json"));
  const xcode = JSON.parse(fs.readFileSync(capacitorRequire.resolve("xcode/package.json"), "utf8"));
  assertAtLeast(xcode.version, "4.0.0");
  assert.equal(Object.hasOwn(xcode.dependencies ?? {}, "uuid"), false);
});

test("markdownlint resolves js-yaml 5 and loads normal YAML configuration", () => {
  assertAtLeast(markdownlintRequire("js-yaml/package.json").version, "5.4.2");
  const yaml = markdownlintRequire("js-yaml");
  assert.deepEqual(
    yaml.load("default: false\nMD001: true\nMD013:\n  line_length: 120\n"),
    { default: false, MD001: true, MD013: { line_length: 120 } },
  );
});

test("js-yaml limits empty merge sources without timing assertions", () => {
  const yaml = markdownlintRequire("js-yaml");
  const source = "arr: &arr [{}, {}, {}]\ntarget: { <<: *arr }\n";
  assert.throws(
    () => yaml.load(source, { schema: yaml.YAML11_SCHEMA, maxTotalMergeKeys: 2 }),
    /merge/i,
  );
  const oversized = `arr: &arr [${Array(101).fill("{}").join(",")} ]\ntarget: { <<: *arr }\n`;
  assert.throws(() => yaml.load(oversized, { schema: yaml.YAML11_SCHEMA }), /merge/i);
});

test("markdownlint CLI honors YAML and TOML config and reports violations", (t) => {
  const directory = temporaryDirectory(t);
  const document = path.join(directory, "document.md");
  const run = (config, contents) => {
    fs.writeFileSync(document, contents);
    const result = spawnSync(process.execPath, [markdownlintEntry, "--config", config, document], {
      cwd: directory,
      encoding: "utf8",
      timeout: 15000,
    });
    assert.ifError(result.error);
    assert.equal(result.signal, null);
    return result;
  };
  for (const [extension, contents] of [
    ["yaml", "default: false\nMD001: true\n"],
    ["toml", "default = false\nMD001 = true\n"],
  ]) {
    const config = path.join(directory, `config.${extension}`);
    fs.writeFileSync(config, contents);
    assert.equal(run(config, "# Heading\n\n## Subheading\n").status, 0);
    const violation = run(config, "# Heading\n\n### Skipped level\n");
    assert.equal(violation.status, 1);
    assert.match(violation.stderr, /MD001/);
  }
});

test("source-map-js rejects invalid and excessive indexed section offsets", () => {
  const postcssRequire = createRequire(frontendRequire.resolve("postcss"));
  assert.equal(postcssRequire("source-map-js/package.json").version, "1.2.2");
  const { SourceMapConsumer } = postcssRequire("source-map-js");
  const base = { version: 3, sources: ["a.js"], names: [], mappings: "AAAA" };
  const indexed = (line, column = 0, map = base) => ({
    version: 3,
    sections: [{ offset: { line, column }, map }],
  });
  for (const invalid of [-1, 0.5, NaN, Infinity, "1", Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => new SourceMapConsumer(indexed(invalid)), /non-negative integers/);
    assert.throws(() => new SourceMapConsumer(indexed(0, invalid)), /non-negative integers/);
  }
  assert.throws(() => new SourceMapConsumer(indexed(10000001)), /must not exceed/);
  assert.throws(
    () => new SourceMapConsumer(indexed(5000000, 0, indexed(5000001))),
    /including offsets of nested sections/,
  );
  const consumer = new SourceMapConsumer(indexed(2));
  assert.equal(consumer.originalPositionFor({ line: 3, column: 1 }).source, "a.js");
});

test("PostCSS preserves CSS and emits a usable source map", async () => {
  const css = "a { color: red }\n";
  const result = await frontendRequire("postcss")([]).process(css, {
    from: "input.css",
    to: "output.css",
    map: { inline: false, annotation: false },
  });
  assert.equal(result.css, css);
  const postcssRequire = createRequire(frontendRequire.resolve("postcss"));
  const consumer = new (postcssRequire("source-map-js").SourceMapConsumer)(result.map.toJSON());
  assert.equal(consumer.originalPositionFor({ line: 1, column: 0 }).source, "input.css");
});

test("math consumer renders inline and display math with patched KaTeX", async () => {
  const mathEntry = frontendRequire.resolve("micromark-extension-math");
  const mathRequire = createRequire(mathEntry);
  assert.equal(mathRequire("katex").version, "0.18.2");
  const { math, mathHtml } = await import(pathToFileURL(mathEntry).href);
  const { micromark } = await import(pathToFileURL(frontendRequire.resolve("micromark")).href);
  const render = (source, options) =>
    micromark(source, {
      extensions: [math()],
      htmlExtensions: [mathHtml(options)],
    });
  const html = render("$x^2$\n\n$$\n\\frac{1}{2}\n$$\n");
  assert.match(html, /class="math math-inline"/);
  assert.match(html, /class="math math-display"/);
  assert.match(html, /class="katex"/);
  assert.doesNotMatch(html, /katex-error/);
  const link = String.raw`$\href{https://example.com/}{x}$`;
  assert.doesNotMatch(render(link), /<a href=/);
  assert.match(render(link, { trust: true }), /<a href="https:\/\/example.com\/"/);
  const katex = mathRequire("katex");
  assert.doesNotMatch(
    katex.renderToString(String.raw`\href{javascript:alert(1)}{x}`, Object.create({ trust: true })),
    /<a href=/,
  );
});

test("markdownlint and Knip TOML consumers preserve normal parsing", async () => {
  for (const consumer of [markdownlintRequire, createRequire(frontendRequire.resolve("knip"))]) {
    const entry = consumer.resolve("smol-toml");
    const metadata = JSON.parse(
      fs.readFileSync(path.join(path.dirname(entry), "../package.json"), "utf8"),
    );
    assertAtLeast(metadata.version, "1.9.0");
    const { parse } = await import(pathToFileURL(entry).href);
    const config = parse('default = false\nMD001 = true\n[MD013]\nline_length = 120\n');
    assert.equal(Object.getPrototypeOf(config), null);
    assert.deepEqual(
      { ...config, MD013: { ...config.MD013 } },
      { default: false, MD001: true, MD013: { line_length: 120 } },
    );
    const flat = Array.from({ length: 128 }, (_, i) => `k${i} = ${i}\n`).join("");
    assert.deepEqual(
      { ...parse(flat) },
      Object.fromEntries(Array.from({ length: 128 }, (_, i) => [`k${i}`, i])),
    );
    assert.throws(() => parse("key = 1\nkey = 2\n"), /already defined|duplicate/i);
  }
});

test("legacy YAML 4 consumers retain their schema and CommonJS API", () => {
  for (const consumer of ["cosmiconfig", "@redocly/openapi-core"]) {
    const consumerRequire = createRequire(frontendRequire.resolve(consumer));
    assertAtLeast(consumerRequire("js-yaml/package.json").version, "4.3.2");
    const yaml = consumerRequire("js-yaml");
    assert.ok(yaml.DEFAULT_SCHEMA);
    assert.deepEqual(yaml.load("base: &base { enabled: true }\nmerged: { <<: *base }\n"), {
      base: { enabled: true },
      merged: { enabled: true },
    });
  }
});
