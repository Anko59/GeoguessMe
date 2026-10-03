// Network provenance verification; only integrity-verified source is extracted.
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const root = path.resolve(__dirname, "../../..");
const vendor = path.join(root, "frontend/vendor/geoguessme-patched-braces");
const provenance = JSON.parse(fs.readFileSync(path.join(vendor, "provenance.json"), "utf8"));
const hash = (algorithm, bytes, encoding = "hex") => createHash(algorithm).update(bytes).digest(encoding);

function run(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, encoding: "utf8", timeout: 15000 });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
}

async function main() {
  assert.equal(provenance.upstream.tarball, "https://registry.npmjs.org/braces/-/braces-3.0.3.tgz");
  assert.equal(provenance.upstream.integrity,
    "sha512-yQbXgO/OSZVD2IsiLlro+7Hf6Q18EJrKSEsdoMzKePKXct3gvD8oLcOQdIzGupr5Fj+EDe8gO/lxc1BzfMpxvA==");
  const response = await fetch(provenance.upstream.tarball, { signal: AbortSignal.timeout(30000) });
  assert.equal(response.status, 200, `upstream fetch failed: ${response.status}`);
  const bytes = Buffer.from(await response.arrayBuffer());
  assert.ok(bytes.length < 1024 * 1024, "unexpected upstream artifact size");
  assert.equal(`sha512-${hash("sha512", bytes, "base64")}`, provenance.upstream.integrity);
  assert.equal(hash("sha256", fs.readFileSync(path.join(vendor, "security.patch"))),
    provenance.security.localPatchSha256);
  const target = fs.mkdtempSync(path.join(os.tmpdir(), "braces-upstream-"));
  try {
    const archive = path.join(target, "braces.tgz");
    fs.writeFileSync(archive, bytes);
    run("tar", ["-xzf", archive, "-C", target], target);
    const source = path.join(target, "package");
    const upstreamPackage = JSON.parse(fs.readFileSync(path.join(source, "package.json"), "utf8"));
    assert.equal(upstreamPackage.name, "braces");
    assert.equal(upstreamPackage.version, "3.0.3");
    for (const [file, hashes] of Object.entries(provenance.files)) {
      assert.equal(hash("sha256", fs.readFileSync(path.join(source, file))), hashes.upstreamSha256, file);
    }
    run("git", ["apply", "--unidiff-zero", path.join(vendor, "security.patch")], source);
    for (const [file, hashes] of Object.entries(provenance.files)) {
      const reconstructed = fs.readFileSync(path.join(source, file));
      assert.equal(hash("sha256", reconstructed), hashes.patchedSha256, file);
      assert.deepEqual(reconstructed, fs.readFileSync(path.join(vendor, file)), file);
    }
    console.log("braces backport: integrity-pinned upstream artifact + exact source reconstruction PASS");
  } finally {
    fs.rmSync(target, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
