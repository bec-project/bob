// Types of OpenCode's server API, taken from the SDK that matches the image's OpenCode version.
// Only types are imported: requests go through our own small client in opencode.ts.
export type {
  Agent,
  AssistantMessage,
  Event,
  FilePart,
  Message,
  Model,
  Part,
  PatchPart,
  PermissionRequest,
  Provider,
  QuestionInfo,
  QuestionRequest,
  ReasoningPart,
  Session,
  SessionStatus,
  SnapshotFileDiff,
  StepFinishPart,
  TextPart,
  Todo,
  ToolPart,
  ToolState,
  UserMessage,
} from "@opencode-ai/sdk/v2";

import type { Message, Part } from "@opencode-ai/sdk/v2";

/** One message with its parts, as GET /session/{id}/message returns it. */
export interface MessageEntry {
  info: Message;
  parts: Part[];
}

/** What bob prints on stdout once the container is ready (container_manager.Connection). */
export interface BobConnection {
  url: string;
  password: string;
  username?: string;
  container?: string;
  runtime?: string;
  workspace?: string;
  readOnly?: boolean;
  mcpPorts?: Record<string, string>;
}

export interface ModelRef {
  providerID: string;
  modelID: string;
}

/** An MCP server entry from OpenCode's merged configuration. */
export interface McpConfigEntry {
  type?: string;
  url?: string;
  enabled?: boolean;
  headers?: Record<string, string>;
}

export interface McpStatus {
  status: string;
  error?: string;
}
