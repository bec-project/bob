"""Exercise interrupted GitHub release recovery without network access or publication."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from release_checks import verify_checksums


class ReleaseRecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.commands = self.root / "bin"
        self.commands.mkdir()
        self.assets = self.root / "assets"
        self.assets.mkdir()
        self.state = self.root / "state.json"
        self.state.write_text(json.dumps({"draft": True, "notes": "Original release notes"}))
        script = (
            f"#!{sys.executable}\n"
            + r"""
import json, os, pathlib, shutil, sys
root = pathlib.Path(os.environ["TEST_RECOVERY_ROOT"])
args = sys.argv[1:]
name = pathlib.Path(sys.argv[0]).name
state_path = root / "state.json"
state = json.loads(state_path.read_text())
if name == "git":
    if args[0] == "tag":
        assert args == ["tag", "--points-at", "tested-commit", "--list", "v[0-9]*"]
        print(os.environ.get("TEST_TAGS", "v1.2.3"))
    elif args == ["rev-parse", "HEAD"]:
        print(os.environ.get("TEST_HEAD", "tested-commit"))
    else:
        raise AssertionError(args)
elif name == "go":
    if os.environ.get("TEST_BUILD_FAIL") == "1":
        sys.exit(1)
    (root / "built").touch()
    pathlib.Path(args[args.index("-o")+1]).write_bytes(b"synthetic-release-binary")
elif name == "gh":
    if args[0] == "api":
        if os.environ.get("TEST_API_FAIL") == "1":
            sys.exit(1)
        assert args[1:3] == ["--paginate", "repos/owner/bob/releases"]
        if '.tag_name == "v1.2.3"' in args[-1]:
            if "draft" in state:
                print(str(state["draft"]).lower())
        else:
            print(os.environ.get("TEST_PUBLISHED_TAGS", "v1.2.2"))
    else:
        assert args[:1] == ["release"] and args[2] == "v1.2.3"
        if args[1] == "create":
            assert "draft" not in state
            assert "--draft" in args and "--verify-tag" in args
            state = {"draft": True, "notes": "Generated notes"}
        elif args[1] == "upload":
            assert state["draft"] is True
            assert "--clobber" in args
            files = [pathlib.Path(value) for value in args if value.startswith("/")]
            assert len(files) == 5
            for index, source in enumerate(files):
                shutil.copyfile(source, root / "assets" / source.name)
                failure = root / "fail_upload_once"
                if index == 0 and failure.exists():
                    failure.unlink()
                    sys.exit(1)
        elif args[1] == "edit":
            assert state["draft"] is True
            assert len(list((root / "assets").iterdir())) == 5
            assert "--draft=false" in args
            state["draft"] = False
            state["latest"] = "--latest=true" in args
        else:
            raise AssertionError(args)
        state_path.write_text(json.dumps(state))
else:
    raise AssertionError(name)
"""
        )
        for command in ("git", "gh", "go"):
            path = self.commands / command
            path.write_text(script)
            path.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": str(self.commands) + os.pathsep + os.environ["PATH"],
            "GITHUB_SHA": "tested-commit",
            "GITHUB_REPOSITORY": "owner/bob",
            "TEST_RECOVERY_ROOT": str(self.root),
        }

    def run_recovery(self):
        return subprocess.run(
            ["bash", str(Path(__file__).with_name("recover_release.sh"))],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_partial_draft_upload_resumes_on_retry(self):
        (self.root / "fail_upload_once").touch()
        first = self.run_recovery()
        self.assertNotEqual(first.returncode, 0)
        self.assertTrue(json.loads(self.state.read_text())["draft"])
        self.assertEqual(len(list(self.assets.iterdir())), 1)
        second = self.run_recovery()
        self.assertEqual(second.returncode, 0, second.stderr)
        state = json.loads(self.state.read_text())
        self.assertFalse(state["draft"])
        self.assertEqual(state["notes"], "Original release notes")
        verify_checksums(self.assets, "1.2.3")

    def test_tag_without_github_release_is_recovered(self):
        self.state.write_text("{}")
        result = self.run_recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        state = json.loads(self.state.read_text())
        self.assertFalse(state["draft"])
        self.assertTrue(state["latest"])
        verify_checksums(self.assets, "1.2.3")

    def test_published_release_is_not_rebuilt_or_modified(self):
        original = {"draft": False, "notes": "Published notes"}
        self.state.write_text(json.dumps(original))
        result = self.run_recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(self.state.read_text()), original)
        self.assertFalse((self.root / "built").exists())
        self.assertEqual(list(self.assets.iterdir()), [])

    def test_untagged_commit_is_left_for_semantic_release(self):
        self.env["TEST_TAGS"] = ""
        result = self.run_recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / "built").exists())

    def test_api_failure_is_not_treated_as_missing_release(self):
        self.env["TEST_API_FAIL"] = "1"
        result = self.run_recovery()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "built").exists())
        self.assertTrue(json.loads(self.state.read_text())["draft"])

    def test_wrong_checkout_is_not_used_to_rebuild_assets(self):
        self.env["TEST_HEAD"] = "another-commit"
        result = self.run_recovery()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("CI-tested commit", result.stderr)
        self.assertFalse((self.root / "built").exists())

    def test_build_failure_does_not_publish_draft(self):
        self.env["TEST_BUILD_FAIL"] = "1"
        result = self.run_recovery()
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(json.loads(self.state.read_text())["draft"])
        self.assertEqual(list(self.assets.iterdir()), [])

    def test_older_recovery_does_not_mark_github_latest(self):
        self.env["TEST_PUBLISHED_TAGS"] = "v1.2.4"
        result = self.run_recovery()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(json.loads(self.state.read_text())["latest"])
