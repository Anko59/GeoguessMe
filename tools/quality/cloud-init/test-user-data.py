#!/usr/bin/env python3
"""Exercise Terraform's real user-data locals with Ubuntu's native cloud-init.

Run only through the Dockerized Make target. No live cloud, operator state,
root runtime, or runtime network is used: Terraform evaluates its existing locals
in a provider-free temporary module, and the installer runs under temporary prefixes.
The actual Terraform module is also tested with three explicit mock providers,
pre-cached at image build time, offline and without operator inputs or state.
"""

import base64
import email.policy
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import struct
import subprocess
import tempfile
import unittest
import uuid
from email.parser import BytesParser

from cloudinit import handlers, helpers, user_data, util, version

ROOT = Path("/workspace")
MAIN = ROOT / "infra/terraform/main.tf"
INSTALLER = ROOT / "infra/cloud-init/install-runtime-bundle.sh"
FAKE_INPUTS = {
    "var.admin_ssh_public_key": "ssh-ed25519 AAAAoperator operator",
    "var.dev_ci_ssh_public_key": "ssh-ed25519 AAAAdevelopment development",
    "var.production_ci_ssh_public_key": "ssh-ed25519 AAAAproduction production",
    "var.runtime_revision": "b" * 40,
    "data.cloudflare_zero_trust_tunnel_cloudflared_token.app.token": "mock-tunnel-token",
}
BUNDLE_PATH = "/tmp/geoguessme-runtime-bundle"
INSTALLER_PATH = "/usr/local/sbin/geoguessme-install-runtime-bundle"
BOOTSTRAP_PATH = "/usr/local/sbin/geoguessme-bootstrap-host"


def terraform_environment(directory):
    # Do not inherit operator variables, CLI arguments, credentials, or config.
    return {
        "PATH": os.environ["PATH"],
        "HOME": str(directory),
        "TF_IN_AUTOMATION": "1",
        "CHECKPOINT_DISABLE": "1",
        "TF_CLI_CONFIG_FILE": "/dev/null",
        "TF_DATA_DIR": str(directory / ".terraform"),
    }


def mocked_terraform_tests(directory):
    source = ROOT / "infra/terraform"
    for path in source.glob("*.tf"):
        if path.name == "backend.tf":
            continue
        # The production definitions are unchanged except for source-file
        # resolution: paths point to the read-only checkout, not the temp module.
        content = path.read_text(encoding="utf-8").replace("${path.module}", str(source))
        (directory / path.name).write_text(content, encoding="utf-8")
    (directory / ".terraform.lock.hcl").write_bytes((source / ".terraform.lock.hcl").read_bytes())
    tests = directory / "tests"
    tests.mkdir()
    fixture = (source / "tests/config.tftest.hcl").read_text(encoding="utf-8")
    assert set(re.findall(r'mock_provider "([^\"]+)"', fixture)) == {"cloudflare", "hcloud", "random"}
    for provider in ["cloudflare", "hcloud", "random"]:
        assert re.search(rf'mock_provider "{provider}"\s*\{{\s*alias\s*=\s*"mock"', fixture)
        assert re.search(rf'{provider}\s*=\s*{provider}\.mock', fixture)
    assert len(re.findall(r'run "', fixture)) == len(re.findall(r'providers\s*=\s*\{', fixture)) == 1
    fixture = fixture.replace('"../cloud-init/', '"' + str(ROOT / "infra/cloud-init") + "/")
    (tests / "config.tftest.hcl").write_text(fixture, encoding="utf-8")
    assert not list(directory.glob("*.tfvars")) and not (directory / "backend.tf").exists()
    environment = terraform_environment(directory)
    # The image caches these exact existing providers; no registry/network is
    # consulted at runtime, and no `terraform apply` command is ever executed.
    for arguments in [
        ["init", "-backend=false", "-input=false", "-lockfile=readonly", "-plugin-dir=/opt/terraform-providers"],
        ["validate", "-no-color"],
        ["test", "-no-color"],
    ]:
        subprocess.run(["terraform", *arguments], cwd=directory, env=environment, check=True)


def realistic_inputs():
    # Deterministic fake public bytes only: no private keys or live token exist.
    inputs = dict(FAKE_INPUTS)
    algorithm = b"ssh-ed25519"
    for variable, label in [
        ("var.admin_ssh_public_key", "ops-operator"),
        ("var.dev_ci_ssh_public_key", "development-ci"),
        ("var.production_ci_ssh_public_key", "production-ci"),
    ]:
        public = hashlib.sha256(("cloud-init fake public key: " + label).encode()).digest()
        wire = struct.pack(">I", len(algorithm)) + algorithm + struct.pack(">I", len(public)) + public
        inputs[variable] = "ssh-ed25519 " + base64.b64encode(wire).decode() + " " + label + "@example.test"
    token = {
        "a": hashlib.sha256(b"cloud-init fake account").hexdigest()[:32],
        "t": str(uuid.UUID(bytes=hashlib.sha256(b"cloud-init fake tunnel").digest()[:16], version=4)),
        "s": base64.b64encode(hashlib.sha256(b"cloud-init fake secret").digest()).decode(),
    }
    inputs["data.cloudflare_zero_trust_tunnel_cloudflared_token.app.token"] = base64.b64encode(
        json.dumps(token, separators=(",", ":")).encode()
    ).decode()
    return inputs


def render(directory, utf8=False, realistic=False):
    """Evaluate the actual locals, substituting only fake provider/key inputs."""
    source = MAIN.read_text(encoding="utf-8")
    assert source.count("  user_data          = local.runtime_user_data") == 1
    body = source.split("\nlocals {\n", 1)[1].split(
        '\n}\n\nresource "random_bytes"', 1
    )[0]
    member_array = body.split("  runtime_bundle_files = [", 1)[1].split("\n  ]", 1)[0]
    relative_paths = re.findall(r'file\("\$\{path.module\}/([^\"]+)"\)', member_array)
    assert len(relative_paths) == 33, "The complete original 32 + appended config are required"
    paths = [(ROOT / "infra/terraform" / path).resolve() for path in relative_paths]
    assert len(set(paths)) == 33 and paths[-1] == ROOT / "deployment/s3-fixture/credentials.json"
    members = [path.read_bytes() for path in paths]
    assert all(member.endswith(b"\n") for member in members)
    assert not members[-1].endswith(b"\n\n"), "Literal YAML clip chomping needs one final newline"
    body = body.replace("${path.module}", str(ROOT / "infra/terraform"))
    inputs = realistic_inputs() if realistic else FAKE_INPUTS
    for name, value in inputs.items():
        assert body.count(name) == 1, f"Unexpected input use: {name}"
        body = body.replace(name, json.dumps(value))
    if utf8:
        # Exercise the real Terraform byte-length expression, not a Python copy.
        fixture = json.loads(members[-1])
        fixture["utf8_probe"] = "é-測試-🌍\n  literal indentation\n"
        members[-1] = (json.dumps(fixture, ensure_ascii=False) + "\n").encode("utf-8")
        probe = directory / "utf8.json"
        probe.write_bytes(members[-1])
        old = str(ROOT / "infra/terraform" / relative_paths[-1])
        assert body.count(old) == 1
        body = body.replace(old, str(probe))
    (directory / "main.tf").write_text("locals {\n" + body + "\n}\n", encoding="utf-8")
    expression = "jsonencode({user_data=local.runtime_user_data, cloud_config=local.runtime_cloud_config, source_config=local.runtime_cloud_config_template, bundle=local.runtime_bundle, installer=local.runtime_installer, members=local.runtime_bundle_files})"
    result = subprocess.run(
        ["terraform", "console", "-no-color"],
        cwd=directory,
        input=expression + "\n",
        text=True,
        encoding="utf-8",
        capture_output=True,
        check=True,
        env=terraform_environment(directory),
    )
    rendered = json.loads(json.loads(result.stdout))
    rendered["inputs"] = inputs
    assert [member.encode("utf-8") for member in rendered["members"]] == members
    groups = [
        (ROOT / "deployment/scripts/hosted", "/opt/geoguessme/bin/", "0755", 12),
        (ROOT / "deployment", "/opt/geoguessme/config/", "0444", 3),
        (ROOT / "deployment/watch", "/opt/geoguessme/config/watch/", "0444", 3),
        (ROOT / "infra/cloud-init/units", "/etc/systemd/system/", "0644", 14),
        (ROOT / "deployment/s3-fixture", "/opt/geoguessme/config/s3-fixture/", "0644", 1),
    ]
    for source, _target, _mode, count in groups:
        assert sum(path.parent == source for path in paths) == count
    rendered["destinations"] = []
    for path in paths:
        group = next(group for group in groups if path.parent == group[0])
        rendered["destinations"].append(group[1] + path.name + " " + group[2])
    return rendered, members


def process(blob, expected_cloud_config, directory):
    """Only the native processor decides whether this is a cloud-config part."""
    # Python's email parser reports malformed MIME even when cloud-init tolerates
    # a missing separator. A malformed producer must fail, not pass on tolerance.
    mime = BytesParser(policy=email.policy.default).parsebytes(blob)
    assert not mime.defects, "Malformed MIME framing"
    processor = user_data.UserDataProcessor(helpers.Paths({"cloud_dir": str(directory)}))
    processed = processor.process(blob)
    parts = [part for part in processed.walk() if not part.is_multipart()]
    assert len(parts) == 1, "Must dispatch exactly one complete cloud-config"
    assert parts[0].get_content_type() == "text/cloud-config", "Wrong native dispatch type"
    # Use the same decoded payload API as the actual native handler walker;
    # choosing a convenient email decode path must not conceal dispatch damage.
    payload = util.fully_decoded_payload(parts[0])
    assert isinstance(payload, str)
    assert payload == expected_cloud_config.decode("utf-8"), "Native decoded cloud-config corrupted UTF-8"
    dispatched = []
    handlers.walk(processed, lambda _data, _filename, content, headers: dispatched.append((content, headers)), None)
    assert len(dispatched) == 1 and dispatched[0][1]["Content-Type"] == "text/cloud-config"
    content = dispatched[0][0]
    assert content == expected_cloud_config.decode("utf-8"), "Native cloud-config dispatch corrupted UTF-8"
    return util.load_yaml(content)


def inspect_config(config, rendered, members):
    inputs = rendered["inputs"]
    assert config["package_update"] is True and config["package_upgrade"] is True
    assert config["packages"] == [
        "age", "ca-certificates", "curl", "docker.io", "docker-compose-v2",
        "gzip", "unattended-upgrades", "ufw",
    ]
    assert config["users"] == [
        {"name": "ops", "groups": ["sudo", "docker"], "shell": "/bin/bash",
         "sudo": ["ALL=(ALL) NOPASSWD:ALL"], "lock_passwd": True},
        {"name": "deploy", "groups": ["docker"], "shell": "/bin/sh", "lock_passwd": True},
    ]
    files = {entry["path"]: entry for entry in config["write_files"]}
    assert len(files) == len(config["write_files"]) == 11
    assert files["/etc/ssh/sshd_config.d/00-geoguessme.conf"]["content"] == (
        "PasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin no\nAllowUsers ops deploy\n"
    )
    assert files["/home/ops/.ssh/authorized_keys"]["content"] == inputs["var.admin_ssh_public_key"] + "\n"
    assert files["/home/deploy/.ssh/authorized_keys"]["content"] == (
        'restrict,command="/opt/geoguessme/bin/forced-command.sh dev" '
        + inputs["var.dev_ci_ssh_public_key"] + "\n"
        + 'restrict,command="/opt/geoguessme/bin/forced-command.sh production" '
        + inputs["var.production_ci_ssh_public_key"] + "\n"
    )
    for path in ["/home/ops/.ssh/authorized_keys", "/home/deploy/.ssh/authorized_keys"]:
        assert files[path]["defer"] is True and files[path]["permissions"] == "0600"
    assert files["/etc/cloudflared/token"]["content"] == inputs["data.cloudflare_zero_trust_tunnel_cloudflared_token.app.token"] + "\n"
    assert files["/etc/cloudflared/token"]["permissions"] == "0600"
    assert files["/opt/geoguessme/config/runtime-revision"]["content"] == "b" * 40 + "\n"
    pins = json.loads((ROOT / "deployment/images/host-tools.json").read_text())["cloudflared"]
    commands = config["runcmd"]
    assert commands == [[BOOTSTRAP_PATH, pins["version"], pins["debSha256"]]]
    bootstrap_entry = files[BOOTSTRAP_PATH]
    assert bootstrap_entry["content"].encode("utf-8") == (ROOT / "infra/cloud-init/bootstrap-host.sh").read_bytes()
    assert bootstrap_entry["owner"] == "root:root" and bootstrap_entry["permissions"] == "0700"
    assert "encoding" not in bootstrap_entry, "Nested bootstrap compression is forbidden"
    assert config["final_message"] == "GeoGuessMe host bootstrap complete"
    bundle_entry, installer_entry = files[BUNDLE_PATH], files[INSTALLER_PATH]
    for entry, mode in [(bundle_entry, "0600"), (installer_entry, "0700")]:
        assert entry["owner"] == "root:root" and entry["permissions"] == mode
        assert "encoding" not in entry, "Nested compression is forbidden"
    bundle = bundle_entry["content"].encode("utf-8")
    installer = installer_entry["content"].encode("utf-8")
    assert bundle == rendered["bundle"].encode("utf-8")
    assert installer == rendered["installer"].encode("utf-8") == INSTALLER.read_bytes()
    assert installer.endswith(b"\n") and not installer.endswith(b"\n\n")
    magic, remainder = bundle.split(b"\n", 1)
    assert magic == b"GEOGUESSME_RUNTIME_BUNDLE_V1"
    lengths = []
    for _ in range(33):
        length, remainder = remainder.split(b"\n", 1)
        assert length.isdigit() and int(length) > 0
        lengths.append(int(length))
    assert lengths == [len(member) for member in members]
    for length, member in zip(lengths, members):
        assert remainder[:length] == member, "Framed member boundary or native UTF-8 bytes changed"
        remainder = remainder[length:]
    assert not remainder, "Unexpected trailing bytes after the 33rd member"
    return bundle, installer


def install_and_check(directory, bundle, installer, members, expected_destinations):
    """Run the decoded installer with all absolute root destinations sandboxed."""
    root = directory / "root"
    config = root / "opt/geoguessme/config"
    config.mkdir(parents=True)
    text = installer.decode("utf-8")
    destinations = text.split("<<'FILES'\n", 1)[1].split("\nFILES", 1)[0].splitlines()
    assert len(destinations) == 33
    assert destinations == expected_destinations, "Installer destinations no longer match their canonical source fields"
    rewritten = text.replace("/opt/geoguessme", str(root / "opt/geoguessme")).replace(
        "/etc/systemd/system", str(root / "etc/systemd/system")
    )
    script, bundle_file = directory / "installer.sh", directory / "bundle"
    script.write_text(rewritten, encoding="utf-8")
    bundle_file.write_bytes(bundle)
    subprocess.run(["sh", str(script), str(bundle_file)], check=True, capture_output=True)
    expected_manifest = []
    for destination, member in zip(destinations, members):
        path, mode = destination.split()
        installed = root / path.lstrip("/")
        assert installed.read_bytes() == member
        metadata = installed.stat()
        assert metadata.st_uid == metadata.st_gid == 0
        assert stat.S_IMODE(metadata.st_mode) == int(mode, 8)
        relative = "units/" + installed.name if path.startswith("/etc/") else path.removeprefix("/opt/geoguessme/")
        expected_manifest.append(hashlib.sha256(member).hexdigest() + "  " + relative + "\n")
    manifest = config / "runtime-hashes"
    assert manifest.read_bytes() == "".join(expected_manifest).encode("utf-8")
    metadata = manifest.stat()
    assert metadata.st_uid == metadata.st_gid == 0 and stat.S_IMODE(metadata.st_mode) == 0o444
    assert not list(config.glob("runtime-stage.*")) and not list(config.glob("runtime-hashes.*"))


def reject(blob, expected, directory, label):
    try:
        process(blob, expected, directory)
    except (AssertionError, RuntimeError, util.DecompressionError, UnicodeError, ValueError):
        return
    raise AssertionError(f"{label} was accepted or silently fell back")


def verify(rendered, members, directory):
    blob = rendered["user_data"].encode("utf-8")
    mime = BytesParser(policy=email.policy.default).parsebytes(blob)
    assert not mime.defects and mime["MIME-Version"] == "1.0"
    assert mime.get_content_type() == "application/gzip"
    assert mime["Content-Transfer-Encoding"] == "base64"
    header, body = blob.split(b"\n\n", 1)
    assert all(0 < len(line) <= 76 for line in body.splitlines())
    compressed = base64.b64decode(b"".join(body.splitlines()), validate=True)
    assert compressed[:2] == b"\x1f\x8b"
    expected = rendered["cloud_config"].encode("utf-8")
    config = process(blob, expected, directory)
    assert config == util.load_yaml(rendered["source_config"]), "Compact cloud-config changed source value types or fields"
    bundle, installer = inspect_config(config, rendered, members)
    install_and_check(directory, bundle, installer, members, rendered["destinations"])
    # These use the native processing/dispatch path, never a hand-written fallback.
    corrupt = bytearray(compressed)
    corrupt[-8] ^= 1  # Corrupt the gzip CRC while keeping MIME/base64 valid.
    for label, payload in [("corrupt gzip", corrupt), ("truncated gzip", compressed[:-8])]:
        reject(header + b"\n\n" + base64.b64encode(payload) + b"\n", expected, directory, label)
    reject(blob.replace(b"application/gzip", b"application/octet-stream", 1), expected, directory, "bad MIME type")
    reject(blob.replace(b"Content-Transfer-Encoding: base64", b"Content-Transfer-Encoding: 7bit", 1), expected, directory, "bad MIME encoding")
    reject(blob.replace(b"\n\n", b"\n", 1), expected, directory, "missing MIME separator")
    return len(blob)


class CloudInitTests(unittest.TestCase):
    def test_native_user_data(self):
        self.assertEqual(os.geteuid(), 0, "The disposable container must test root-owned installation")
        with tempfile.TemporaryDirectory(prefix="geoguessme-cloud-init-") as temporary:
            root = Path(temporary)
            sizes = {}
            for name, utf8, realistic in [
                ("current", False, False), ("utf8", True, False),
                ("realistic", False, True), ("realistic_utf8", True, True),
            ]:
                directory = root / name
                directory.mkdir()
                rendered, members = render(directory, utf8, realistic)
                sizes[name] = verify(rendered, members, directory)
            print(f"Native cloud-init {version.version_string()}: MIME UTF-8 bytes={sizes}; margin bytes="
                  f"{ {name: 32768 - size for name, size in sizes.items()} }; strict cap=32768; "
                  "33 exact members, root installer/manifest, and 5 corruption cases each.", flush=True)
            self.assertTrue(all(size <= 32768 for size in sizes.values()), f"Strict Hetzner byte cap exceeded: {sizes} bytes")

    def test_mocked_terraform_user_data(self):
        with tempfile.TemporaryDirectory(prefix="geoguessme-mocked-terraform-") as temporary:
            mocked_terraform_tests(Path(temporary))


if __name__ == "__main__":
    unittest.main(verbosity=2)
