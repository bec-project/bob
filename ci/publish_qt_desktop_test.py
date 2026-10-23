"""Exercise the Qt desktop app's delivery using fake GitHub and registry commands."""

import base64
import hashlib
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

GENERIC = "https://gitea.psi.ch/api/packages/bec/generic/bob/1.2.3/"
RPM_UPLOAD = "https://gitea.psi.ch/api/packages/bec/rpm/upload"
RPM_PACKAGE = "https://gitea.psi.ch/api/packages/bec/rpm/package/bob-gui/1.2.3-1.el9/x86_64"


class QtDesktopDeliveryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.bundles = self.root / "dist"
        self.generic = self.root / "generic"
        self.rpm = self.root / "rpm"
        self.github = self.root / "github"
        self.commands = self.root / "bin"
        for directory in (self.bundles, self.generic, self.rpm, self.github, self.commands):
            directory.mkdir()
        self.rpm_name = "bob-gui-1.2.3-1.el9.x86_64.rpm"
        self.names = [self.rpm_name, "bob-gui_1.2.3_darwin_arm64.dmg"]
        for name in self.names:
            (self.bundles / name).write_bytes(name.encode())
        token = base64.b64encode(b"user:synthetic-token").decode()
        script = (
            f"#!{sys.executable}\n"
            + r"""
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ["TEST_RELEASE_ROOT"])
args = sys.argv[1:]
name = pathlib.Path(sys.argv[0]).name
if name == "gh":
    assert args[:3] == ["release", args[1], "v1.2.3"] and "owner/bob" in args, args
    if args[1] == "view":
        print(os.environ.get("TEST_DRAFT", "false"))
    elif args[1] == "upload":
        assert "--clobber" in args
        for asset in args[args.index("--clobber") + 1:]:
            shutil.copyfile(asset, root / "github" / pathlib.Path(asset).name)
    else:
        raise AssertionError(args)
elif name == "curl":
"""
            + f"    assert sys.stdin.read() == 'header = \"Authorization: Basic {token}\"\\n'\n"
            + f"    generic, upload, package = {GENERIC!r}, {RPM_UPLOAD!r}, {RPM_PACKAGE!r}\n"
            + r"""    assert not any("synthetic-token" in argument for argument in args)
    url = args[-1]
    if url == upload or url == package:
        registry = root / "rpm" / "bob-gui.rpm"
    else:
        assert url.startswith(generic), url
        registry = root / "generic" / url[len(generic):]
    if "--upload-file" in args:
        failure = root / "fail_once"
        if failure.exists() and url.endswith(".dmg"):
            failure.unlink()
            print("503", end="")
        elif registry.exists():
            print("409", end="")
        else:
            shutil.copyfile(args[args.index("--upload-file") + 1], registry)
            print("201", end="")
    else:
        shutil.copyfile(registry, args[args.index("--output") + 1])
else:
    raise AssertionError(name)
"""
        )
        for command in ("gh", "curl"):
            path = self.commands / command
            path.write_text(script)
            path.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": str(self.commands) + os.pathsep + os.environ["PATH"],
            "GITHUB_REPOSITORY": "owner/bob",
            "GITEA_REGISTRY_TOKEN": "user:synthetic-token",
            "TEST_RELEASE_ROOT": str(self.root),
        }

    def run_delivery(self):
        return subprocess.run(
            [
                "bash",
                str(Path(__file__).with_name("publish_qt_desktop.sh")),
                "1.2.3",
                str(self.bundles),
            ],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_delivers_every_bundle_with_checksums(self):
        result = self.run_delivery()
        self.assertEqual(result.returncode, 0, result.stderr)
        sums = "bob-gui_1.2.3_SHA256SUMS"
        expected = sorted([*self.names, sums])
        self.assertEqual(sorted(p.name for p in self.generic.iterdir()), expected)
        self.assertEqual(sorted(p.name for p in self.github.iterdir()), expected)
        self.assertEqual((self.rpm / "bob-gui.rpm").read_bytes(), self.rpm_name.encode())
        manifest = (self.github / sums).read_text()
        for name in self.names:
            digest = hashlib.sha256(name.encode()).hexdigest()
            self.assertIn(f"{digest}  {name}\n", manifest)

    def test_retry_after_partial_upload(self):
        (self.root / "fail_once").touch()
        first = self.run_delivery()
        self.assertNotEqual(first.returncode, 0)
        self.assertIn("HTTP 503", first.stderr)
        self.assertEqual(list(self.github.iterdir()), [])
        second = self.run_delivery()
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(len(list(self.github.iterdir())), 3)
        third = self.run_delivery()
        self.assertEqual(third.returncode, 0, third.stderr)

    def test_different_rpm_in_the_registry_stops_before_github(self):
        (self.rpm / "bob-gui.rpm").write_bytes(b"another build")
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Existing registry asset differs", result.stderr)
        self.assertEqual(list(self.github.iterdir()), [])

    def test_draft_release_is_rejected(self):
        self.env["TEST_DRAFT"] = "true"
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.generic.iterdir()), [])

    def test_missing_rpm_is_rejected(self):
        (self.bundles / self.rpm_name).unlink()
        result = self.run_delivery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("one RPM", result.stderr)
        self.assertEqual(list(self.generic.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
