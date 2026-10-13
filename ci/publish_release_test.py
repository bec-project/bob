"""Exercise release delivery using fake GitHub and registry commands, without publishing."""

import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class ReleaseDeliveryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.assets = self.root / "assets"
        self.registry = self.root / "registry"
        self.commands = self.root / "bin"
        for directory in (self.assets, self.registry, self.commands):
            directory.mkdir()
        self.names = [
            f"bob-manager_1.2.3_{target}.tar.gz"
            for target in ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")
        ]
        for name in self.names:
            (self.assets / name).write_bytes(name.encode())
        self.write_checksums()
        script = (
            f"#!{sys.executable}\n"
            + r"""
import base64, os, pathlib, shutil, sys
root = pathlib.Path(os.environ["TEST_RELEASE_ROOT"])
args = sys.argv[1:]
name = pathlib.Path(sys.argv[0]).name
if name == "git":
    assert args == ["tag", "--points-at", "tested-commit", "--list", "v[0-9]*"]
    print(os.environ.get("TEST_TAGS", "v1.2.3"))
elif name == "gh":
    if args[0] == "api":
        assert args[1:3] == ["--paginate", "repos/owner/bob/releases"]
        if os.environ.get("TEST_RELEASE_API_FAIL") == "1":
            sys.exit(1)
        print(os.environ.get("TEST_STABLE_TAGS", "v1.2.3"))
        sys.exit(0)
    assert "v1.2.3" in args
    if args[1] == "view":
        print(os.environ.get("TEST_DRAFT", "false"))
    elif args[1] == "download":
        destination = pathlib.Path(args[args.index("--dir") + 1])
        assert args.count("--pattern") == 5
        for asset in (root / "assets").iterdir():
            shutil.copyfile(asset, destination / asset.name)
    else:
        raise AssertionError(args)
elif name == "curl":
    expected = base64.b64encode(b"user:synthetic-token").decode()
    assert sys.stdin.read() == f'header = "Authorization: Basic {expected}"\n'
    assert not any("synthetic-token" in argument for argument in args)
    asset = args[-1].rsplit("/", 1)[1]
    registry = root / "registry" / asset
    if "--upload-file" in args:
        failure = root / "fail_once"
        if failure.exists() and asset.endswith("linux_arm64.tar.gz"):
            failure.unlink()
            print("503", end="")
        elif registry.exists():
            print("409", end="")
        else:
            shutil.copyfile(args[args.index("--upload-file") + 1], registry)
            print("201", end="")
    else:
        shutil.copyfile(registry, args[args.index("--output") + 1])
elif name == "docker":
    assert args[:5] == ["buildx", "imagetools", "create", "--tag", "gitea.psi.ch/bec/bob:latest"]
    (root / "latest_image").write_text(args[5])
else:
    raise AssertionError(name)
"""
        )
        for command in ("git", "gh", "curl", "docker"):
            path = self.commands / command
            path.write_text(script)
            path.chmod(0o755)
        self.output = self.root / "output"
        self.env = {
            **os.environ,
            "PATH": str(self.commands) + os.pathsep + os.environ["PATH"],
            "GITHUB_SHA": "tested-commit",
            "GITHUB_REPOSITORY": "owner/bob",
            "GITHUB_OUTPUT": str(self.output),
            "GITEA_REGISTRY_TOKEN": "user:synthetic-token",
            "TEST_RELEASE_ROOT": str(self.root),
        }

    def write_checksums(self):
        manifest = "".join(
            f"{hashlib.sha256((self.assets / name).read_bytes()).hexdigest()}  {name}\n"
            for name in self.names
        )
        (self.assets / "SHA256SUMS").write_text(manifest)

    def run_delivery(self):
        self.output.write_text("")
        return subprocess.run(
            ["bash", str(Path(__file__).with_name("publish_release.sh"))],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_retry_after_partial_upload_on_fresh_runner(self):
        (self.root / "fail_once").touch()
        first = self.run_delivery()
        self.assertNotEqual(first.returncode, 0)
        self.assertIn("HTTP 503", first.stderr)
        self.assertNotIn("release_available=true", self.output.read_text())
        self.assertTrue((self.registry / self.names[0]).exists())
        second = self.run_delivery()
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(self.output.read_text(), "release_available=true\nversion=1.2.3\n")
        self.assertEqual(len(list(self.registry.iterdir())), 5)
        third = self.run_delivery()
        self.assertEqual(third.returncode, 0, third.stderr)

    def test_existing_conflicting_file_is_rejected(self):
        shutil.copytree(self.assets, self.registry, dirs_exist_ok=True)
        (self.registry / self.names[0]).write_bytes(b"different release")
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Existing registry asset differs", result.stderr)
        self.assertNotIn("release_available=true", self.output.read_text())

    def test_untagged_commit_skips_delivery(self):
        self.env["TEST_TAGS"] = ""
        result = self.run_delivery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.output.read_text(), "release_available=false\n")
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_draft_release_is_rejected(self):
        self.env["TEST_DRAFT"] = "true"
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_corrupted_github_asset_is_rejected_before_upload(self):
        (self.assets / self.names[0]).write_bytes(b"corrupt")
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_ambiguous_release_tags_are_rejected(self):
        self.env["TEST_TAGS"] = "v1.2.3\nv1.2.4"
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Multiple stable releases", result.stderr)
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_omitted_checksum_rejects_unchecked_corrupt_archive(self):
        manifest = self.assets / "SHA256SUMS"
        manifest.write_text("".join(manifest.read_text().splitlines(keepends=True)[1:]))
        (self.assets / self.names[0]).write_bytes(b"unchecked corruption")
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("each expected archive exactly once", result.stderr)
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_duplicate_checksum_entry_is_rejected(self):
        manifest = self.assets / "SHA256SUMS"
        content = manifest.read_text()
        manifest.write_text(content + content.splitlines(keepends=True)[0])
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.registry.iterdir()), [])

    def test_unexpected_checksum_filename_is_rejected(self):
        manifest = self.assets / "SHA256SUMS"
        manifest.write_text(manifest.read_text().replace(self.names[0], "../outside.tar.gz"))
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.registry.iterdir()), [])

    def run_promotion(self, version="1.2.3"):
        return subprocess.run(
            ["bash", str(Path(__file__).with_name("promote_image.sh")), version],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_old_release_retry_preserves_newer_latest(self):
        self.env["TEST_STABLE_TAGS"] = "v1.2.3\nv1.2.4"
        latest = self.root / "latest_image"
        latest.write_text("gitea.psi.ch/bec/bob:1.2.4")
        result = self.run_promotion()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(latest.read_text(), "gitea.psi.ch/bec/bob:1.2.4")

    def test_newest_stable_release_is_promoted(self):
        self.env["TEST_STABLE_TAGS"] = "v1.2.2\nv1.2.3\nv1.3.0-rc.1"
        result = self.run_promotion()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / "latest_image").read_text(), "gitea.psi.ch/bec/bob:1.2.3")

    def test_latest_comparison_is_numeric(self):
        self.env["TEST_STABLE_TAGS"] = "v1.2.9\nv1.2.10"
        result = self.run_promotion("1.2.10")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / "latest_image").read_text(), "gitea.psi.ch/bec/bob:1.2.10")

    def test_release_listing_failure_does_not_promote(self):
        self.env["TEST_RELEASE_API_FAIL"] = "1"
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "latest_image").exists())

    def test_unpublished_version_is_not_promoted(self):
        self.env["TEST_STABLE_TAGS"] = "v1.2.2"
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "latest_image").exists())


if __name__ == "__main__":
    unittest.main()
