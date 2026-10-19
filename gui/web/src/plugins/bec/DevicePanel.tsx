import { useCallback, useEffect, useMemo, useState } from "react";
import { IconRefresh } from "../../components/Icons";
import type { PluginContext } from "../types";
import { buildCard, type Card } from "./cards";
import { ReportCard } from "./ReportCard";
import { callResult as call, useBecClient } from "./server";

interface Device {
  name: string;
  type?: string;
  device_class?: string;
  readout_priority?: string;
  enabled?: boolean;
}

export function DevicePanel({ context }: { context: PluginContext }) {
  const { client: mcp, url, source } = useBecClient(context);
  const [devices, setDevices] = useState<Device[]>([]);
  const [summary, setSummary] = useState<{ text: string; tone: string }>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<string>();
  const [reading, setReading] = useState<{ device: string; card?: Card; error?: string; busy: boolean }>();

  const load = useCallback(async () => {
    if (!mcp) return;
    setLoading(true);
    setError(undefined);
    try {
      const ping = await call(mcp, "bec_ping", {}).catch((e: Error) => ({ ok: false, summary: e.message }));
      setSummary({ text: ping.summary, tone: ping.ok === false ? "error" : "ok" });
      const all: Device[] = [];
      let offset: number | null = 0;
      while (offset !== null && all.length < 5000) {
        const page = await call(mcp, "bec_devices", { limit: 500, offset });
        const data = (page.data ?? {}) as { devices?: Device[]; next_offset?: number | null };
        all.push(...(data.devices ?? []));
        offset = typeof data.next_offset === "number" ? data.next_offset : null;
      }
      setDevices(all);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [mcp]);

  const read = useCallback(
    async (device: string) => {
      if (!mcp) return;
      setSelected(device);
      setReading({ device, busy: true });
      try {
        const result = await call(mcp, "bec_device_read", { device });
        setReading({ device, card: buildCard("bec_device_read", result), busy: false });
      } catch (e) {
        setReading({ device, error: (e as Error).message, busy: false });
      }
    },
    [mcp],
  );

  useEffect(() => {
    setDevices([]);
    setSelected(undefined);
    setReading(undefined);
    void load();
  }, [load]);

  const shown = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    const list = needle
      ? devices.filter((d) => d.name.toLowerCase().includes(needle) || (d.device_class ?? "").toLowerCase().includes(needle))
      : devices;
    return list.slice(0, 400);
  }, [devices, filter]);

  if (!url) {
    return (
      <div className="panel-message">
        No BEC MCP server found. Add one to bob.toml as <code>[mcp.bec_mcp]</code>, or set its URL in
        Settings → Plugins → BEC.
      </div>
    );
  }

  return (
    <>
      <div className="device-status" title={`${url} (from ${source})`}>
        {loading ? <span className="spinner" /> : <span className={`dot ${summary?.tone === "ok" ? "ok" : error || summary?.tone === "error" ? "error" : ""}`} />}
        <span style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {error ? "Cannot reach the BEC MCP server" : summary?.text ?? "Connecting…"}
        </span>
        <button className="icon-btn" title="Reload devices" onClick={() => void load()} disabled={loading}>
          <IconRefresh size={14} />
        </button>
      </div>
      <div className="devices-tools">
        <input
          className="input"
          placeholder={devices.length ? `Filter ${devices.length} devices` : "Filter devices"}
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
      </div>
      {error && <div className="panel-message error">{error}</div>}
      {shown.map((device) => (
        <div key={device.name}>
          <button
            className={`device-row ${selected === device.name ? "selected" : ""}`}
            onClick={() => (selected === device.name ? setSelected(undefined) : void read(device.name))}
            title={device.device_class}
          >
            <span className={`dot ${device.enabled === false ? "warn" : "ok"}`} />
            <span className="name">{device.name}</span>
            <span className="kind">{device.type ?? ""}</span>
          </button>
          {selected === device.name && reading?.device === device.name && (
            <div className="device-detail">
              {reading.busy && <div className="working"><span className="spinner" /> Reading {device.name}…</div>}
              {reading.error && <div className="panel-message error">{reading.error}</div>}
              {reading.card && <ReportCard card={reading.card} compact />}
              <div className="row">
                <button className="btn small" onClick={() => void read(device.name)} disabled={reading.busy}>
                  Read again
                </button>
                <button className="btn small" onClick={() => context.insertPrompt(`What is the state of ${device.name}? `)}>
                  Ask about it
                </button>
                <button className="btn small" onClick={() => context.insertPrompt(`Diagnose the device ${device.name}.`)}>
                  Diagnose
                </button>
              </div>
            </div>
          )}
        </div>
      ))}
      {!loading && !error && devices.length > 0 && shown.length === 0 && (
        <div className="panel-message">No device matches “{filter}”.</div>
      )}
      {!loading && !error && devices.length === 0 && <div className="panel-message">BEC has no devices loaded.</div>}
    </>
  );
}
