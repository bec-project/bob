"""Opt-in real macOS sandbox tests using the shared synthetic provider/MCP scenario.

Set BOB_SANDBOX_TEST_MANAGER, BOB_SANDBOX_TEST_OPENCODE and BOB_SANDBOX_TEST_SRT
as for sandbox_linux_test.py. The manager must include macOS sandbox support.
"""

import json
import os
import shutil
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import sandbox_linux_test as shared


class SyntheticProxy(BaseHTTPRequestHandler):
    """Answer only the reserved test destination, without forwarding or resolving it."""

    protocol_version = "HTTP/1.1"

    def setup(self):
        super().setup()
        self.connection.settimeout(5)

    def log_message(self, *_args):
        pass

    def do_CONNECT(self):
        if self.path != "provider.invalid:80":
            self.send_error(403)
            return
        self.send_response(200)
        self.end_headers()
        self.close_connection = False

    def do_GET(self):
        if self.path not in ("http://provider.invalid/probe", "/probe"):
            self.send_error(403)
            return
        self.server.requests.append(self.path)
        body = b"proxy probe complete"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.close_connection = True


@unittest.skipUnless(
    sys.platform == "darwin" and all(os.environ.get(name) for name in shared.VARIABLES),
    "requires macOS and BOB_SANDBOX_TEST_MANAGER/OPENCODE/SRT",
)
class MacOSSandboxTests(shared.LinuxSandboxTests):
    __unittest_skip__ = False
    __unittest_skip_why__ = ""

    def test_helper_reaches_synthetic_proxy_without_localhost_dns(self):
        reply = self.start()
        self.assertIn("connection", reply, f"sandbox startup failed: {reply}\n{self.output()}")
        node = shutil.which("node")
        self.assertIsNotNone(node, "srt requires Node.js")
        proxy = ThreadingHTTPServer(("127.0.0.1", 0), SyntheticProxy)
        proxy.daemon_threads = True
        proxy.requests = []
        thread = threading.Thread(
            target=proxy.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True
        )
        self.servers.append((proxy, thread))
        thread.start()

        (settings_file,) = self.runtime.glob("bob-*/srt-settings.json")
        settings = json.loads(settings_file.read_text())
        # Srt's documented external proxy setting advertises localhost to its child.
        # The fake proxy answers a reserved, non-loopback name so NO_PROXY cannot
        # bypass this path. No DNS lookup or connection to a real provider is needed.
        settings["network"]["httpProxyPort"] = proxy.server_port
        settings_file = self.root / "proxy-settings.json"
        settings_file.write_text(json.dumps(settings))
        home = self.home.resolve()
        environment = {
            "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
            "HOME": str(home / ".opencode-policy/home"),
            "XDG_CONFIG_HOME": str(home / ".opencode-policy/config"),
            "XDG_CACHE_HOME": str(home / ".cache"),
            "XDG_DATA_HOME": str(home / ".local/share"),
            "XDG_STATE_HOME": str(home / ".local/state"),
            "TMPDIR": str(home / ".cache/tmp"),
            "CLAUDE_CODE_TMPDIR": str(home / ".cache/tmp"),
            # Run a fetch probe using the pinned executable's embedded Bun runtime.
            "BUN_BE_BUN": "1",
        }
        probe = (
            "const proxy = new URL(process.env.HTTP_PROXY); "
            "if (proxy.hostname !== '127.0.0.1') throw new Error('proxy still needs DNS'); "
            "const response = await fetch('http://provider.invalid/probe', "
            "{signal: AbortSignal.timeout(5000)}); "
            "if (!response.ok) throw new Error('proxy request failed'); "
            "console.log(await response.text());"
        )
        process = subprocess.Popen(
            [
                node,
                str(self.srt),
                "--settings",
                str(settings_file),
                "--",
                str(self.manager),
                "--sandbox-helper",
                str(self.opencode),
                "-e",
                probe,
            ],
            cwd=home / ".opencode-policy/config/opencode",
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            start_new_session=True,
        )
        try:
            output, _ = process.communicate(timeout=20)
            self.assertEqual(process.returncode, 0, output)
            self.assertIn("proxy probe complete", output)
            self.assertEqual(len(proxy.requests), 1)
        finally:
            shared.stop_process(process, timeout=5)
            process.stdout.close()

    @unittest.skip("macOS uses Seatbelt rather than Linux seccomp")
    def test_missing_seccomp_helper_refuses_to_start_opencode(self):
        pass


if __name__ == "__main__":
    unittest.main()
