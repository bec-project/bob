"""Keep the packaged OpenCode release aligned with the runtime's tested security policy."""

import re
import unittest
from pathlib import Path


class SandboxPackageTest(unittest.TestCase):
    def test_packaged_opencode_matches_the_policy_version(self):
        root = Path(__file__).resolve().parents[1]
        build = (root / "ci" / "build_qt_rpm.sh").read_text()
        runtime = (root / "launcher" / "sandbox" / "sandbox_tools.go").read_text()
        packaged = re.search(r"^opencode_version=([0-9.]+)$", build, re.MULTILINE)
        supported = re.search(r'^const pinnedOpenCodeVersion = "([0-9.]+)"$', runtime, re.MULTILINE)
        self.assertIsNotNone(packaged, "RPM build must pin OpenCode")
        self.assertIsNotNone(supported, "runtime must pin its tested OpenCode policy")
        self.assertEqual(packaged.group(1), supported.group(1))


if __name__ == "__main__":
    unittest.main()
