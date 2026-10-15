#!/usr/bin/env node
// Joins the user's shared bob (starting it when none runs), waits for its connection line, then
// starts the GUI connected to it. Ctrl-C (or closing the desktop window) stops the GUI and ends
// its hold on bob; bob stops 90 seconds after the last app that uses it is gone.
//
//   npm --prefix gui run bob -- [--config bob.toml] [--desktop] [--no-open]
//
// The connection (it holds the OpenCode password) goes to a private temporary file that only
// this user can read; the GUI reads it through BOB_CONNECTION_FILE and the file is deleted on exit.

import { spawn, spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import readline from "node:readline";
import { fileURLToPath } from "node:url";

const gui = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const repo = path.dirname(gui);
// npm runs scripts in gui/; INIT_CWD is where the user typed the command.
const userDir = process.env.INIT_CWD ?? process.cwd();

function usage(code) {
  console.log(`Usage: npm --prefix gui run bob -- [options]

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
    cwd: repo,
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
const consumer = options.desktop ? "gui-desktop-dev" : "gui-web";
const bob = spawn(bobBinary, ["--shared", "--consumer", consumer, "--config", config], {
  cwd: userDir,
  stdio: ["ignore", "pipe", "inherit"],
});
let bobExit = null;
const bobDone = new Promise((resolve) =>
  bob.on("exit", (code, signal) => {
    bobExit = code ?? (signal ? 1 : 0);
    resolve(bobExit);
  }),
);
bob.on("error", (error) => console.error(`bob-gui: cannot start bob: ${error.message}`));

let guiProcess = null;
let stopping = false;

async function stop(reason) {
  if (stopping) return;
  stopping = true;
  if (reason) console.error(`bob-gui: ${reason}; stopping`);
  if (guiProcess && guiProcess.exitCode === null) guiProcess.kill("SIGTERM");
  // SIGINT ends this consumer's hold; the broker stops bob once nothing else holds it.
  if (bobExit === null) bob.kill("SIGINT");
  const code = await bobDone;
  removeSecret();
  process.exit(code);
}

for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) process.on(signal, () => void stop());
process.on("exit", removeSecret);

function startGui() {
  const env = { ...process.env, BOB_CONNECTION_FILE: connectionFile };
  const bin = (name) => path.join(gui, "node_modules", ".bin", name);
  if (options.desktop) {
    console.error("bob-gui: starting the desktop app (the first build compiles Rust and takes a while)");
    guiProcess = spawn(bin("tauri"), ["dev"], { cwd: gui, env, stdio: "inherit" });
  } else {
    const args = options.open ? ["--open"] : [];
    guiProcess = spawn(bin("vite"), args, { cwd: gui, env, stdio: "inherit" });
  }
  guiProcess.on("exit", () => void stop(options.desktop ? "the desktop app closed" : "the GUI server exited"));
}

const lines = readline.createInterface({ input: bob.stdout });
let connected = false;
lines.on("line", (line) => {
  if (connected) {
    console.log(line);
    return;
  }
  let connection;
  try {
    connection = JSON.parse(line);
  } catch {
    console.log(line);
    return;
  }
  if (typeof connection?.url !== "string" || typeof connection?.password !== "string") {
    console.log(line);
    return;
  }
  connected = true;
  writeFileSync(connectionFile, `${JSON.stringify(connection)}\n`, { mode: 0o600 });
  console.error(`bob-gui: OpenCode is up at ${connection.url} (container ${connection.container ?? "?"})`);
  startGui();
});

bobDone.then((code) => {
  if (!stopping) void stop(connected ? `bob exited (${code})` : `bob exited before it was ready (${code})`);
});
