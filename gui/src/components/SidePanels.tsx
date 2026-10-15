import type { ReactNode } from "react";
import { useEnabledPlugins, usePluginContext } from "../plugins/registry";
import type { GuiPlugin, PanelSpec } from "../plugins/types";
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

function PluginPanel({ plugin, panel }: { plugin: GuiPlugin; panel: PanelSpec }) {
  const context = usePluginContext(plugin);
  return (
    <PanelFrame title={panel.title} icon={panel.icon} width={panel.width}>
      {panel.render(context)}
    </PanelFrame>
  );
}

const CHANGES = { key: "core/changes", title: "Workspace changes", icon: <IconFile size={18} /> };

/** The panel rail on the right, and the open panel: workspace changes, then plugin panels. */
export function SidePanels() {
  const plugins = useEnabledPlugins();
  const open = useStore((s) => s.prefs.panel);
  const setPrefs = useStore((s) => s.setPrefs);
  const changed = useStore((s) =>
    s.currentId ? new Set((s.changes[s.currentId] ?? []).flatMap((c) => c.files.map((f) => f.file))).size : 0,
  );
  const panels = plugins.flatMap((plugin) =>
    (plugin.panels ?? []).map((panel) => ({ plugin, panel, key: `${plugin.id}/${panel.id}` })),
  );
  const active = panels.find((p) => p.key === open);
  const rail = [
    { key: CHANGES.key, title: CHANGES.title, icon: CHANGES.icon, badge: changed },
    ...panels.map(({ key, panel }) => ({ key, title: panel.title, icon: panel.icon, badge: 0 })),
  ];
  return (
    <>
      {open === CHANGES.key && (
        <PanelFrame title={CHANGES.title} icon={CHANGES.icon} width={460}>
          <ChangesPanel />
        </PanelFrame>
      )}
      {active && <PluginPanel plugin={active.plugin} panel={active.panel} />}
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
