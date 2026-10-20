import type { ReactNode } from "react";
import { useStore } from "../state/store";
import { ChangesPanel } from "./ChangesPanel";
import { IconFile, IconX } from "./Icons";

function PanelFrame({ title, icon, width, children }: { title: string; icon: ReactNode; width?: number; children: ReactNode }) {
  const setPrefs = useStore((s) => s.setPrefs);
  return (
    <aside className="panel" style={width ? { width } : undefined}>
      <div className="panel-head">
        {icon}
        <span className="grow">{title}</span>
        <button className="icon-btn" title="Close" onClick={() => setPrefs({ panel: undefined })}>
          <IconX size={15} />
        </button>
      </div>
      <div className="panel-body">{children}</div>
    </aside>
  );
}

const CHANGES = { key: "core/changes", title: "Workspace changes", icon: <IconFile size={18} /> };

/** The panel rail on the right, and the open panel. */
export function SidePanels() {
  const open = useStore((s) => s.prefs.panel);
  const setPrefs = useStore((s) => s.setPrefs);
  const changed = useStore((s) =>
    s.currentId ? new Set((s.changes[s.currentId] ?? []).flatMap((c) => c.files.map((f) => f.file))).size : 0,
  );
  const rail = [{ key: CHANGES.key, title: CHANGES.title, icon: CHANGES.icon, badge: changed }];
  return (
    <>
      {open === CHANGES.key && (
        <PanelFrame title={CHANGES.title} icon={CHANGES.icon} width={460}>
          <ChangesPanel />
        </PanelFrame>
      )}
      <nav className="rail">
        {rail.map((item) => (
          <button
            key={item.key}
            className={`icon-btn rail-btn ${open === item.key ? "active" : ""}`}
            title={item.title}
            onClick={() => setPrefs({ panel: open === item.key ? undefined : item.key })}
          >
            {item.icon}
            {item.badge > 0 && <span className="rail-badge">{item.badge}</span>}
          </button>
        ))}
      </nav>
    </>
  );
}
