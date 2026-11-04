"""Regression tests for ownership and descendant cleanup in the sandbox test harness."""

import os
import signal
import subprocess
import sys
import time
import unittest
from unittest.mock import Mock, patch

import sandbox_linux_test as shared


class ProcessCleanupTests(unittest.TestCase):
    def test_macos_snapshot_excludes_unrelated_processes(self):
        result = Mock(
            stdout=(
                "10 1 Wed Nov 4 10:00:00 2026\n"
                "11 10 Wed Nov 4 10:00:01 2026\n"
                "12 11 Wed Nov 4 10:00:02 2026\n"
                "99 1 Wed Nov 4 10:00:03 2026\n"
            )
        )
        with patch.object(shared.subprocess, "run", return_value=result):
            self.assertEqual(
                shared.macos_process_tree(10),
                {
                    10: "Wed Nov 4 10:00:00 2026",
                    11: "Wed Nov 4 10:00:01 2026",
                    12: "Wed Nov 4 10:00:02 2026",
                },
            )
            self.assertEqual(shared.macos_process_tree(100), {})

    def test_cleanup_does_not_signal_reused_pid(self):
        process = Mock(pid=10, returncode=None)
        process.poll.return_value = 0
        snapshots = [{10: "original", 11: "original child"}, {11: "new process"}, {}]
        with (
            patch.object(shared, "process_tree", side_effect=snapshots),
            patch.object(shared.os, "kill") as kill,
        ):
            shared.stop_process(process)
        kill.assert_not_called()

    def test_cleanup_does_not_reinspect_reaped_parent(self):
        process = Mock(returncode=0)
        with (
            patch.object(shared, "process_tree") as inspect,
            patch.object(shared.os, "kill") as kill,
        ):
            shared.stop_process(process)
        inspect.assert_not_called()
        kill.assert_not_called()

    def test_failed_inspection_still_stops_owned_parent(self):
        process = Mock(pid=10, returncode=None)
        process.poll.return_value = None
        with (
            patch.object(shared, "process_tree", side_effect=OSError("inspection failed")),
            patch.object(shared.os, "kill") as kill,
            self.assertRaisesRegex(OSError, "inspection failed"),
        ):
            shared.stop_process(process)
        process.terminate.assert_called_once()
        process.wait.assert_called_once()
        kill.assert_not_called()

    @unittest.skipUnless(sys.platform == "darwin", "requires macOS process inspection")
    def test_cleanup_stops_child_in_separate_process_group(self):
        command = (
            "import subprocess,sys,time; "
            "child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)'], "
            "start_new_session=True); print(child.pid,flush=True); time.sleep(30)"
        )
        process = subprocess.Popen(
            [sys.executable, "-c", command],
            stdout=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
        child = None
        identity = None
        try:
            child = int(process.stdout.readline())
            identity = shared.macos_process_tree(child)[child]
            self.assertIn(child, shared.process_tree(process.pid))
            shared.stop_process(process)
            deadline = time.monotonic() + 2
            while True:
                state = subprocess.run(
                    ["/bin/ps", "-p", str(child), "-o", "stat="],
                    capture_output=True,
                    text=True,
                    timeout=5,
                    check=False,
                ).stdout.strip()
                if not state or state.startswith("Z") or time.monotonic() >= deadline:
                    break
                time.sleep(0.01)
            self.assertTrue(not state or state.startswith("Z"), f"child survived: {state}")
        finally:
            if child is not None and shared.macos_process_tree(child).get(child) == identity:
                try:
                    os.kill(child, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            if process.poll() is None:
                process.kill()
            process.wait(timeout=5)
            process.stdout.close()


if __name__ == "__main__":
    unittest.main()
