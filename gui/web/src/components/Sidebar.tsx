import { useMemo, useState } from "react";
import type { Session } from "../api/types";
import { sortedSessions } from "../state/sessionData";
import { useStore } from "../state/store";
import { dayGroup, relativeTime, useNow } from "./hooks";
import { IconEdit, IconLogo, IconPlus, IconSettings, IconTrash } from "./Icons";
import { useResizable } from "./Splitter";

function SessionRow({ session, active }: { session: Session; active: boolean }) {
  const busy = useStore((s) => s.status[session.id]?.type === "busy" || s.status[session.id]?.type === "retry");
  const waiting = useStore((s) =>
    Object.values(s.permissions).some((p) => p.sessionID === session.id) ||
    Object.values(s.questions).some((q) => q.sessionID === session.id),
  );
  const select = useStore((s) => s.selectSession);
  const rename = useStore((s) => s.renameSession);
  const remove = useStore((s) => s.deleteSession);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(session.title);

  if (editing) {
    return (
      <div className={`session-item ${active ? "active" : ""}`}>
        <input
          autoFocus
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onBlur={() => setEditing(false)}
          onKeyDown={(event) => {
            if (event.key === "Escape") setEditing(false);
            if (event.key === "Enter") {
              setEditing(false);
              if (title.trim() && title !== session.title) void rename(session.id, title.trim());
            }
          }}
        />
      </div>
    );
  }
  return (
    <button
      className={`session-item ${active ? "active" : ""}`}
      onClick={() => select(session.id)}
      onDoubleClick={() => {
        setTitle(session.title);
        setEditing(true);
      }}
      title={session.title}
    >
      {waiting ? (
        <span className="dot warn pulse" title="Waiting for your answer" />
      ) : busy ? (
        <span className="spinner" style={{ width: 12, height: 12 }} />
      ) : null}
      <span className="title">{session.title || "Untitled"}</span>
      <span className="when">{relativeTime(session.time.updated)}</span>
      <span className="actions">
        <span
          className="icon-btn"
          role="button"
          title="Rename"
          onClick={(event) => {
            event.stopPropagation();
            setTitle(session.title);
            setEditing(true);
          }}
        >
          <IconEdit size={14} />
        </span>
        <span
          className="icon-btn"
          role="button"
          title="Delete"
          onClick={(event) => {
            event.stopPropagation();
            if (window.confirm(`Delete “${session.title}”? This removes it from the container's history.`)) {
              void remove(session.id);
            }
          }}
        >
          <IconTrash size={14} />
        </span>
      </span>
    </button>
  );
}

export function Sidebar({ onSettings }: { onSettings: () => void }) {
  const sessions = useStore((s) => s.sessions);
  const currentId = useStore((s) => s.currentId);
  const select = useStore((s) => s.selectSession);
  const stream = useStore((s) => s.stream);
  const connection = useStore((s) => s.connection);
  const version = useStore((s) => s.serverVersion);
  const [query, setQuery] = useState("");
  const { width, splitter } = useResizable("sidebar", 264, "right", 200);
  useNow(true, 30_000);

  const groups = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const list = sortedSessions(sessions).filter((s) => !needle || s.title.toLowerCase().includes(needle));
    const out: { label: string; items: Session[] }[] = [];
    for (const session of list) {
      const label = dayGroup(session.time.updated);
      const last = out[out.length - 1];
      if (last?.label === label) last.items.push(session);
      else out.push({ label, items: [session] });
    }
    return out;
  }, [sessions, query]);

  const streamDot = stream === "open" ? "ok" : stream === "retrying" ? "warn pulse" : "error";
  const streamText =
    stream === "open" ? "Connected" : stream === "retrying" ? "Reconnecting…" : stream === "connecting" ? "Connecting…" : "Offline";

  return (
    <aside className="sidebar" style={{ width }}>
      {splitter}
      <div className="sidebar-head">
        <span className="brand">
          <IconLogo size={20} /> bob
        </span>
        <span style={{ flex: 1 }} />
        <button className="btn small" onClick={() => select(undefined)} title="New chat (Ctrl+Shift+O)">
          <IconPlus size={14} /> New chat
        </button>
      </div>
      <div className="sidebar-tools">
        <input className="input" placeholder="Search chats" value={query} onChange={(event) => setQuery(event.target.value)} />
      </div>
      <div className="session-list">
        {groups.length === 0 && (
          <div className="empty-list">{query ? "No chat matches." : "No chats in this container yet."}</div>
        )}
        {groups.map((group) => (
          <div key={group.label}>
            <div className="session-group">{group.label}</div>
            {group.items.map((session) => (
              <SessionRow key={session.id} session={session} active={session.id === currentId} />
            ))}
          </div>
        ))}
      </div>
      <div className="sidebar-foot">
        <span className={`dot ${streamDot}`} />
        <span className="grow" title={`${connection?.url ?? ""}${connection?.container ? ` · ${connection.container}` : ""}`}>
          {streamText}
          {connection?.container ? ` · ${connection.container}` : ""}
          {version ? ` · OpenCode ${version}` : ""}
        </span>
        <button className="icon-btn" title="Settings" onClick={onSettings}>
          <IconSettings size={16} />
        </button>
      </div>
    </aside>
  );
}
