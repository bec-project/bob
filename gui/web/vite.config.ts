/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { bobServer } from "./server/bob-server";

// The dev and preview servers also answer /__bob/* (see server/bob-server.ts): the connection
// that scripts/bob-gui.mjs captured from bob, and a loopback-only relay for MCP servers, which
// send no CORS headers. A Tauri build reaches both through Rust commands instead.
export default defineConfig({
  plugins: [react(), bobServer()],
  clearScreen: false,
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
  },
  preview: {
    host: "127.0.0.1",
    port: 4173,
    strictPort: true,
  },
  envPrefix: ["VITE_", "TAURI_ENV_"],
  build: {
    target: "es2022",
    sourcemap: true,
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts", "server/**/*.test.ts"],
  },
});
