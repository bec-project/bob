import type { ReactNode } from "react";
import type { McpConfigEntry, PermissionRequest, ToolPart } from "../api/types";

export type Tone = "neutral" | "ok" | "info" | "warn" | "error" | "accent";

/** What a plugin can do with the chat around it. */
export interface PluginContext {
  /** Puts text into the composer and focuses it, without sending. */
  insertPrompt(text: string): void;
  /** Sends a prompt in the open session (creating one if needed). */
  sendPrompt(text: string): void;
  /** MCP servers from OpenCode's configuration, by name. */
  mcpServers: Record<string, McpConfigEntry>;
  settings: Record<string, unknown>;
  setSettings(patch: Record<string, unknown>): void;
}

/** How a plugin wants one tool call drawn. Every field is optional. */
export interface ToolView {
  /** Replaces the tool name on the row, e.g. "Reading the device". */
  label?: string;
  /** One line shown on the row once the call finished. */
  summary?: string;
  tone?: Tone;
  /** Drawn under the row, open by default (a report card instead of raw output). */
  body?: ReactNode;
}

/** Lets a permission view answer the request itself, e.g. after running an edited call. */
export interface PermissionControls {
  /** Replies to OpenCode; a message on "reject" reaches the model as the reason. */
  reply(reply: "once" | "always" | "reject", message?: string): Promise<void>;
}

export interface PermissionView {
  title?: string;
  body?: ReactNode | ((controls: PermissionControls) => ReactNode);
}

export interface PanelSpec {
  id: string;
  title: string;
  /** A short glyph or icon for the panel rail. */
  icon: ReactNode;
  /** Width in pixels; 340 when left out. */
  width?: number;
  render(context: PluginContext): ReactNode;
}

/**
 * A GUI plugin. Plugins only add views; the chat works without any of them. Each one can be
 * switched off in Settings, and starts off unless defaultEnabled is set.
 */
export interface GuiPlugin {
  id: string;
  name: string;
  description: string;
  defaultEnabled: boolean;
  describeTool?(part: ToolPart, context: PluginContext): ToolView | undefined;
  describePermission?(
    request: PermissionRequest,
    tool: ToolPart | undefined,
    context: PluginContext,
  ): PermissionView | undefined;
  panels?: PanelSpec[];
  /** Prompts offered on an empty chat. */
  suggestions?: string[];
  /** Extra controls in the plugin's row in Settings. */
  settingsView?(context: PluginContext): ReactNode;
}
