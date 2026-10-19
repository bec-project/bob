import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore } from "../state/store";
import { IconSend, IconStop, IconX } from "./Icons";

export function Composer() {
  const currentId = useStore((s) => s.currentId);
  const busy = useStore((s) => (s.currentId ? (s.status[s.currentId]?.type ?? "idle") !== "idle" : false));
  const ready = useStore((s) => s.stream === "open" || s.stream === "retrying");
  const send = useStore((s) => s.send);
  const abort = useStore((s) => s.abort);
  const request = useStore((s) => s.composer);
  const notice = useStore((s) => s.notice);
  const dismissNotice = useStore((s) => s.dismissNotice);
  const [text, setText] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    area.current?.focus();
  }, [currentId]);

  useEffect(() => {
    if (!request) return;
    setText((current) =>
      request.replace || !current.trim() ? request.text : `${current.trimEnd()}\n${request.text}`,
    );
    requestAnimationFrame(() => {
      const el = area.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(el.value.length, el.value.length);
    });
  }, [request]);

  useLayoutEffect(() => {
    const el = area.current;
    if (!el) return;
    const fit = () => {
      el.style.height = "";
      if (el.value) el.style.height = `${Math.min(el.scrollHeight, 260)}px`;
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(el.parentElement ?? el);
    return () => observer.disconnect();
  }, [text]);

  const submit = () => {
    const value = text.trim();
    if (!value || !ready) return;
    setText("");
    void send(value);
  };

  return (
    <div className="composer-wrap">
      {notice && (
        <div className={`notice ${notice.tone}`}>
          <span className="grow">{notice.text}</span>
          <button className="icon-btn" onClick={dismissNotice} title="Dismiss">
            <IconX size={14} />
          </button>
        </div>
      )}
      <div className="composer">
        <textarea
          ref={area}
          rows={1}
          placeholder={busy ? "Queue a follow-up…" : "Ask anything"}
          value={text}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              submit();
            } else if (event.key === "Escape" && busy) {
              event.preventDefault();
              void abort();
            }
          }}
        />
        <div className="composer-bar">
          <span className="grow muted" style={{ fontSize: 12 }}>
            {!ready ? "Waiting for the server…" : ""}
          </span>
          {busy && (
            <button className="btn small" onClick={() => void abort()} title="Stop (Esc)">
              <IconStop size={13} /> Stop
            </button>
          )}
          <button className="btn small primary" onClick={submit} disabled={!text.trim() || !ready} title="Send (Enter)">
            <IconSend size={14} /> Send
          </button>
        </div>
      </div>
      <div className="composer-hint">
        <kbd>Enter</kbd> send · <kbd>Shift</kbd>+<kbd>Enter</kbd> new line · <kbd>Esc</kbd> stop
      </div>
    </div>
  );
}
