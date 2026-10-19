import type { OpencodeApi } from "./opencode";
import type { Event } from "./types";

export type StreamState = "connecting" | "open" | "retrying" | "closed";

export interface EventStreamHandlers {
  onEvent(event: Event): void;
  /** The stream (re)opened: anything sent while it was down has to be fetched again. */
  onOpen(reconnected: boolean): void;
  onState(state: StreamState, detail?: string): void;
  /** The server refused the password (bob restarted with a new one): the stream stops. */
  onUnauthorized(): void;
}

/** Splits a text/event-stream body into the JSON payloads of its data fields. */
export class SseParser {
  private buffer = "";
  private data: string[] = [];

  push(chunk: string): string[] {
    this.buffer += chunk;
    const out: string[] = [];
    let newline: number;
    while ((newline = this.buffer.search(/\r?\n/)) !== -1) {
      const line = this.buffer.slice(0, newline);
      this.buffer = this.buffer.slice(newline + (this.buffer[newline] === "\r" ? 2 : 1));
      if (line === "") {
        if (this.data.length) out.push(this.data.join("\n"));
        this.data = [];
      } else if (line.startsWith("data:")) {
        this.data.push(line.slice(line[5] === " " ? 6 : 5));
      }
    }
    return out;
  }
}

const HEARTBEAT_TIMEOUT_MS = 45_000;

/**
 * Follows GET /event. EventSource cannot send an Authorization header, so the stream is read
 * with fetch. It reconnects with backoff, and treats a silent connection (no heartbeat) as lost.
 */
export function followEvents(api: OpencodeApi, handlers: EventStreamHandlers): () => void {
  let stopped = false;
  let controller: AbortController | null = null;
  let opened = false;

  const run = async () => {
    let delay = 500;
    while (!stopped) {
      handlers.onState(opened ? "retrying" : "connecting");
      controller = new AbortController();
      let watchdog: ReturnType<typeof setTimeout> | undefined;
      const arm = () => {
        clearTimeout(watchdog);
        watchdog = setTimeout(() => controller?.abort(), HEARTBEAT_TIMEOUT_MS);
      };
      try {
        arm();
        const response = await fetch(`${api.base}/event`, {
          headers: { Authorization: api.auth, Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (response.status === 401) {
          stopped = true;
          handlers.onUnauthorized();
          break;
        }
        if (!response.ok || !response.body) throw new Error(`event stream failed (${response.status})`);
        handlers.onOpen(opened);
        opened = true;
        delay = 500;
        handlers.onState("open");
        const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
        const parser = new SseParser();
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          arm();
          for (const data of parser.push(value)) {
            try {
              handlers.onEvent(JSON.parse(data) as Event);
            } catch (error) {
              console.warn("bad event", error, data.slice(0, 200));
            }
          }
        }
      } catch (error) {
        if (stopped) break;
        handlers.onState("retrying", (error as Error).message);
      } finally {
        clearTimeout(watchdog);
      }
      if (stopped) break;
      await new Promise((resolve) => setTimeout(resolve, delay));
      delay = Math.min(delay * 2, 10_000);
    }
    handlers.onState("closed");
  };
  void run();
  return () => {
    stopped = true;
    controller?.abort();
  };
}
