import { useState } from "react";
import type { SnapshotFileDiff } from "../api/types";
import { useStore } from "../state/store";
import { IconChevron, IconUndo } from "./Icons";

/** A unified diff with added and removed lines coloured; file headers are left out. */
export function Patch({ patch }: { patch: string }) {
  const lines = patch.split("\n").filter((line) => !/^(diff --git|index |--- |\+\+\+ |=+$|Index: )/.test(line));
  return (
    <pre className="patch">
      {lines.map((line, i) => (
        <div
          key={i}
          className={line.startsWith("@@") ? "hunk" : line.startsWith("+") ? "add" : line.startsWith("-") ? "del" : ""}
        >
          {line || " "}
        </div>
      ))}
    </pre>
  );
}

function FileDiff({ diff }: { diff: SnapshotFileDiff }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="file-diff">
      <button className="file-diff-head" onClick={() => setOpen(!open)} disabled={!diff.patch}>
        <IconChevron size={12} className="chev" style={{ transform: open ? "rotate(90deg)" : undefined }} />
        <span className="name" title={diff.file}>
          {diff.file ?? "(unnamed)"}
        </span>
        {diff.status && diff.status !== "modified" && <span className="chip">{diff.status}</span>}
        <span className="tone-ok">+{diff.additions}</span>
        <span className="tone-error">−{diff.deletions}</span>
      </button>
      {open && diff.patch && <Patch patch={diff.patch} />}
    </div>
  );
}

/** The files a chat's prompts changed in the workspace, newest prompt first, each with Undo. */
export function ChangesPanel() {
  const changes = useStore((s) => (s.currentId ? s.changes[s.currentId] : undefined));
  const currentId = useStore((s) => s.currentId);
  const busy = useStore((s) => (s.currentId ? (s.status[s.currentId]?.type ?? "idle") !== "idle" : false));
  const revertID = useStore((s) => (s.currentId ? s.sessions[s.currentId]?.revert?.messageID : undefined));
  const revert = useStore((s) => s.revert);
  if (!currentId) return <div className="panel-message">Open a chat to see the files it changed.</div>;
  if (!changes?.length) {
    return (
      <div className="panel-message">
        This chat has not changed any file in the workspace. Changes show here as the agent edits them, when the
        workspace is a git repository (OpenCode snapshots it with git).
      </div>
    );
  }
  const files = new Set(changes.flatMap((c) => c.files.map((f) => f.file)));
  return (
    <>
      <div className="device-status">
        {files.size} {files.size === 1 ? "file" : "files"} changed by {changes.length}{" "}
        {changes.length === 1 ? "message" : "messages"}
      </div>
      {[...changes].reverse().map((change) => {
        const undone = revertID !== undefined && change.messageID >= revertID;
        return (
          <div key={change.messageID} className={`change-group ${undone ? "undone" : ""}`}>
            <div className="change-head">
              <span className="prompt" title={change.prompt}>
                {change.prompt || "(no text)"}
              </span>
              {undone ? (
                <span className="chip">undone</span>
              ) : (
                <button
                  className="btn small"
                  disabled={busy}
                  title="Undo this message and everything after it, including these file changes"
                  onClick={() => void revert(change.messageID)}
                >
                  <IconUndo size={12} /> Undo
                </button>
              )}
            </div>
            {change.files.map((diff, i) => (
              <FileDiff key={diff.file ?? i} diff={diff} />
            ))}
          </div>
        );
      })}
    </>
  );
}
