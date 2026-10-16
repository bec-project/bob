#!/usr/bin/env node
// Joins the user's bob (starting it when none runs), waits until it is up, then starts the GUI
// connected to it. Ctrl-C (or closing the desktop window) stops the GUI and ends its hold on bob;
// bob stops 90 seconds after the last app that uses it is gone.
//
//   npm --prefix gui/web run bob -- [--config bob.toml] [--desktop] [--no-open]
//
// The connection (it holds the OpenCode password) goes to a private temporary file that only
// this user can read; the GUI reads it through BOB_CONNECTION_FILE and the file is deleted on exit.

import { spawn, spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import net from "node:net";
import { fileURLToPath } from "node:url";

const web = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const repo = path.dirname(path.dirname(web));
// npm runs scripts in gui/web/; INIT_CWD is where the user typed the command.
const userDir = process.env.INIT_CWD ?? process.cwd();

function usage(code) {
  console.log(`Usage: npm --prefix gui/web run bob -- [options]

  --config PATH   bob configuration (default: bob.toml in the current directory)
  --desktop       open the Tauri desktop app (tauri dev) instead of the browser
  --no-open       do not open a browser tab
  --bob PATH      use this container_manager binary instead of building one`);
  process.exit(code);
}

const options = { config: "bob.toml", desktop: false, open: true, bob: "" };
const argv = process.argv.slice(2);
for (let i = 0; i < argv.length; i++) {
  const arg = argv[i];
  if (arg === "--config") options.config = argv[++i] ?? usage(2);
  else if (arg.startsWith("--config=")) options.config = arg.slice(9);
  else if (arg === "--desktop") options.desktop = true;
  else if (arg === "--no-open") options.open = false;
  else if (arg === "--bob") options.bob = argv[++i] ?? usage(2);
  else if (arg === "-h" || arg === "--help") usage(0);
  else {
    console.error(`unknown option ${arg}`);
    usage(2);
  }
}

const config = path.resolve(userDir, options.config);
let bobBinary = options.bob ? path.resolve(userDir, options.bob) : "";
if (!bobBinary) {
  bobBinary = path.join(repo, "build", "bob-manager");
  console.error("bob-gui: building bob…");
  const build = spawnSync("go", ["build", "-o", bobBinary, "./cmd/container_manager"], {
    cwd: path.join(repo, "launcher"),
    stdio: "inherit",
  });
  if (build.error || build.status !== 0) {
    console.error(`bob-gui: go build failed${build.error ? `: ${build.error.message}` : ""}`);
    process.exit(1);
  }
}

const secretDir = mkdtempSync(path.join(tmpdir(), "bob-gui-"));
chmodSync(secretDir, 0o700);
const connectionFile = path.join(secretDir, "connection.json");
const removeSecret = () => rmSync(secretDir, { recursive: true, force: true });

console.error(`bob-gui: joining your bob (starting it with ${config} if none runs)`);
// bob-manager makes sure this user's broker runs and prints its socket; the lease is a
// connection to that socket, held for as long as this script runs.
const located = spawnSync(bobBinary, ["--config", config], { cwd: userDir, encoding: "utf8" });
if (located.error || located.status !== 0) {
  console.error(`bob-gui: bob could not start: ${located.error?.message ?? located.stderr.trim()}`);
  removeSecret();
  process.exit(1);
}
const socket = located.stdout
  .split("\n")
  .map((line) => {
    try {
      return JSON.parse(line).socket;
    } catch {
      return undefined;
    }
  })
  .find((value) => typeof value === "string");
if (!socket) {
  console.error(`bob-gui: bob printed no broker socket: ${located.stdout.trim()}`);
  removeSecret();
  process.exit(1);
}

let guiProcess = null;
let stopping = false;
let connected = false;
const lease = net.createConnection(socket);

function stop(reason, code = 0) {
  if (stopping) return;
  stopping = true;
  if (reason) console.error(`bob-gui: ${reason}; stopping`);
  if (guiProcess && guiProcess.exitCode === null) guiProcess.kill("SIGTERM");
  // Closing the connection ends this app's hold; the broker stops bob once nothing holds it.
  lease.destroy();
  removeSecret();
  process.exit(code);
}

for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) process.on(signal, () => stop());
process.on("exit", removeSecret);

function startGui() {
  const env = { ...process.env, BOB_CONNECTION_FILE: connectionFile };
  const bin = (name) => path.join(web, "node_modules", ".bin", name);
  if (options.desktop) {
    console.error("bob-gui: starting the desktop app (the first build compiles Rust and takes a while)");
    guiProcess = spawn(bin("tauri"), ["dev"], {
      cwd: web,
      env: { ...env, TAURI_APP_PATH: path.join(repo, "gui", "desktop") },
      stdio: "inherit",
    });
  } else {
    const args = options.open ? ["--open"] : [];
    guiProcess = spawn(bin("vite"), args, { cwd: web, env, stdio: "inherit" });
  }
  guiProcess.on("exit", () => stop(options.desktop ? "the desktop app closed" : "the GUI server exited"));
}

const consumer = options.desktop ? "gui-desktop-dev" : "gui-web";
lease.setEncoding("utf8");
lease.on("connect", () => lease.write(`${JSON.stringify({ consumer, pid: process.pid, config })}\n`));
let pending = "";
lease.on("data", (chunk) => {
  if (connected) return;
  pending += chunk;
  const end = pending.indexOf("\n");
  if (end < 0) return;
  let reply;
  try {
    reply = JSON.parse(pending.slice(0, end));
  } catch {
    stop(`bob's broker sent an unexpected reply: ${pending.slice(0, 200)}`, 1);
    return;
  }
  if (reply.error || !reply.connection) {
    stop(`bob did not start: ${reply.error ?? "the broker sent no connection"}`, 1);
    return;
  }
  if (reply.config && reply.config !== config) {
    console.error(`bob-gui: bob already runs for you with ${reply.config}; using it instead of ${config}`);
  }
  connected = true;
  const connection = reply.connection;
  writeFileSync(connectionFile, `${JSON.stringify(connection)}\n`, { mode: 0o600 });
  console.error(`bob-gui: OpenCode is up at ${connection.url} (container ${connection.container ?? "?"})`);
  startGui();
});
lease.on("error", (error) => stop(`lost bob's broker: ${error.message}`, 1));
lease.on("close", () =>
  stop(connected ? "bob stopped: the broker closed the connection" : "the broker closed the connection before bob was up", 1),
);
