// Regression contracts for the scoped security overrides tracked by issue #376.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { createRequire } = require("node:module");
const { spawnSync } = require("node:child_process");
const { test } = require("node:test");
const vm = require("node:vm");

const root = path.resolve(__dirname, "../../..");
const frontendRequire = createRequire(path.join(root, "frontend/package.json"));
const xcodeRequire = createRequire(frontendRequire.resolve("xcode"));
const markdownlintEntry = frontendRequire.resolve("markdownlint-cli");
const markdownlintRequire = createRequire(markdownlintEntry);

function temporaryDirectory(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "npm-overrides-"));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function emptyProject(filename) {
  const project = frontendRequire("xcode").project(filename);
  project.hash = { project: { objects: { PBXGroup: {} } } };
  return project;
}

test("xcode resolves the patched CommonJS uuid and preserves project IDs", (t) => {
  assert.equal(xcodeRequire("uuid/package.json").version, "11.1.1");
  const uuid = xcodeRequire("uuid");
  assert.match(
    uuid.v4(),
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );
  const filename = path.join(temporaryDirectory(t), "project.pbxproj");
  const project = emptyProject(filename);
  const group = project.pbxCreateGroup("Resources", "Resources");
  assert.match(group, /^[0-9A-F]{12}4[0-9A-F]{3}[89AB][0-9A-F]{7}$/);
  assert.deepEqual(project.allUuids(), [group]);
  fs.writeFileSync(filename, project.writeSync());
  const parsed = frontendRequire("xcode").project(filename).parseSync();
  assert.deepEqual(parsed.allUuids(), [group]);
  assert.equal(parsed.getPBXGroupByKey(group).name, "Resources");
});

test("xcode retries an existing project ID using the unchanged v4 API", () => {
  const sourcePath = xcodeRequire.resolve("./lib/pbxProject");
  const source = fs.readFileSync(sourcePath, "utf8");
  const existing = "000000000000400080000000";
  const expected = "111111111111411181111111";
  const values = [
    "00000000-0000-4000-8000-000000000000",
    "11111111-1111-4111-8111-111111111111",
  ];
  let calls = 0;
  const sandbox = {
    module: { exports: {} },
    __dirname: path.dirname(sourcePath),
    require(name) {
      if (name === "uuid") {
        return {
          v4(...args) {
            assert.deepEqual(args, []);
            assert.ok(calls < values.length, "collision retry must terminate");
            return values[calls++];
          },
        };
      }
      return createRequire(sourcePath)(name);
    },
  };
  vm.runInNewContext(source, sandbox, { filename: sourcePath });
  const project = new sandbox.module.exports("unused.pbxproj");
  project.hash = { project: { objects: { PBXGroup: { [existing]: {} } } } };
  assert.equal(project.generateUuid(), expected);
  assert.equal(calls, 2);
});

test("patched uuid rejects partial buffer writes before modifying output", () => {
  const uuid = xcodeRequire("uuid");
  for (const version of ["v3", "v5"]) {
    for (const offset of [-1, 4]) {
      const buffer = new Uint8Array(8).fill(0xaa);
      assert.throws(() => uuid[version]("x", uuid[version].DNS, buffer, offset), RangeError);
      assert.deepEqual(buffer, new Uint8Array(8).fill(0xaa));
    }
  }
  const buffer = new Uint8Array(8).fill(0xbb);
  assert.throws(() => uuid.v6({}, buffer, 4), RangeError);
  assert.deepEqual(buffer, new Uint8Array(8).fill(0xbb));
});

test("markdownlint resolves js-yaml 5 and loads normal YAML configuration", () => {
  assert.equal(markdownlintRequire("js-yaml/package.json").version, "5.4.2");
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

test("markdownlint CLI honors YAML config and reports violations", (t) => {
  const directory = temporaryDirectory(t);
  const config = path.join(directory, "config.yaml");
  const document = path.join(directory, "document.md");
  fs.writeFileSync(config, "default: false\nMD001: true\n");
  const run = (contents) => {
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
  assert.equal(run("# Heading\n\n## Subheading\n").status, 0);
  const violation = run("# Heading\n\n### Skipped level\n");
  assert.equal(violation.status, 1);
  assert.match(violation.stderr, /MD001/);
});

test("legacy YAML 4 consumers retain their schema and CommonJS API", () => {
  for (const consumer of ["cosmiconfig", "@redocly/openapi-core"]) {
    const consumerRequire = createRequire(frontendRequire.resolve(consumer));
    assert.equal(consumerRequire("js-yaml/package.json").version, "4.3.2");
    const yaml = consumerRequire("js-yaml");
    assert.ok(yaml.DEFAULT_SCHEMA);
    assert.deepEqual(yaml.load("base: &base { enabled: true }\nmerged: { <<: *base }\n"), {
      base: { enabled: true },
      merged: { enabled: true },
    });
  }
});
