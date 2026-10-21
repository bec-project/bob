"""A stand-in DEV-mode BEC MCP server for trying the GUI's write paths without a beamline.

It offers bec_run_scan and bec_queue_control, prints every call to stderr and answers like the
real tools, but never talks to BEC. Run it with a Python that has the `mcp` package (2.x), e.g.
the BEC MCP server's venv, and add it to bob.toml next to the real server:

    python fake_bec_dev_mcp.py 8015
    [mcp.bec_dev]
    url = "http://host.docker.internal:8015/mcp"
    plugin = "bec"
"""

import json
import sys

import uvicorn
from mcp.server.mcpserver import MCPServer
from mcp.server.transport_security import TransportSecuritySettings
from mcp.types import ToolAnnotations

WRITE = ToolAnnotations(readOnlyHint=False, destructiveHint=True)
server = MCPServer("fake-bec-dev")
calls = []
counter = {"scan": 41}


@server.tool(
    name="bec_run_scan",
    description="Submit a BEC scan (fake: records it, runs nothing).",
    annotations=WRITE,
)
async def bec_run_scan(
    scan: str,
    args: list | None = None,
    kwargs: dict | None = None,
    wait: bool = True,
    timeout: float = 120.0,
) -> dict:
    counter["scan"] += 1
    calls.append(
        {"tool": "bec_run_scan", "scan": scan, "args": args, "kwargs": kwargs, "wait": wait}
    )
    print(json.dumps(calls[-1]), file=sys.stderr, flush=True)
    return {
        "submitted": True,
        "accepted": True,
        "status": "queued",
        "scan_number": counter["scan"],
        "request_id": "fake-req",
    }


@server.tool(
    name="bec_queue_control",
    description="abort / halt / resume / reset the scan queue (fake).",
    annotations=WRITE,
)
async def bec_queue_control(action: str) -> dict:
    calls.append({"tool": "bec_queue_control", "action": action})
    print(json.dumps(calls[-1]), file=sys.stderr, flush=True)
    return {"action": action, "queue": {"primary": "RUNNING"}}


port = int(sys.argv[1]) if len(sys.argv) > 1 else 8015
app = server.streamable_http_app(
    streamable_http_path="/mcp",
    transport_security=TransportSecuritySettings(
        enable_dns_rebinding_protection=True,
        allowed_hosts=["127.0.0.1:*", "localhost:*", f"host.docker.internal:{port}"],
        allowed_origins=["http://127.0.0.1:*", "http://localhost:*"],
    ),
    host="127.0.0.1",
)
uvicorn.run(app, host="127.0.0.1", port=port, log_level="warning")
