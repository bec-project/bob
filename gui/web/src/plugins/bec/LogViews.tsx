import { useState } from "react";
import { IconChevron } from "../../components/Icons";
import type { FrameView, IncidentView, LogLine } from "./cards";

/** What a log or incident view may offer; each action is optional. */
export interface LogActions {
  /** Put a question about this item into the composer. */
  ask?(text: string): void;
  /** Show the source behind the traceback in this log record. */
  showCode?(record: number): void;
  /** Show the log records around this record. */
  showRecords?(record: number): void;
}

const tone = (value?: string) => (value && value !== "neutral" ? `tone-${value}` : "");

/** Numbered source lines as the BEC code tools print them; the raising line starts with ">". */
export function CodeLines({ code }: { code: string }) {
  return (
    <pre className="code-lines">
      {code.split("\n").map((line, i) => (
        <div key={i} className={line.startsWith(">") ? "hit" : ""}>
          {line}
        </div>
      ))}
    </pre>
  );
}

export function LogRecords({ records, actions }: { records: LogLine[]; actions?: LogActions }) {
  return (
    <div className="log-records">
      {records.map((record, i) => (
        <div key={record.id ?? i} className="log-record">
          <div className="meta">
            {record.id !== undefined && <span className="muted">#{record.id}</span>}
            <span>{record.time}</span>
            <span className={`chip ${tone(record.tone)}`}>{record.level}</span>
            <span className="service">{record.service}</span>
            {record.traceback && record.id !== undefined && actions?.showCode && (
              <button className="btn small ghost" onClick={() => actions.showCode!(record.id!)}>
                Show code
              </button>
            )}
          </div>
          <div className={`message ${tone(record.tone)}`}>{record.message}</div>
          {record.traceback && (
            <div className="tb">
              {record.traceback.exception}
              {record.traceback.origin && <span className="muted"> at {record.traceback.origin}</span>}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function Incident({ incident, actions }: { incident: IncidentView; actions?: LogActions }) {
  const [open, setOpen] = useState(false);
  const when = incident.first === incident.last || !incident.last
    ? incident.first.slice(11, 19)
    : `${incident.first.slice(11, 19)}–${incident.last.slice(11, 19)}`;
  return (
    <div className={`incident ${tone(incident.tone)}`}>
      <div className="incident-head">
        <span className="ref">{incident.ref}</span>
        <span className="title">{incident.title}</span>
        {incident.count > 1 && <span className="chip">×{incident.count}</span>}
        <span className="muted when">{when}</span>
      </div>
      <div className="incident-message">{incident.message}</div>
      {incident.origin && (
        <div className="incident-origin">
          <span className="muted">raised at</span> <code>{incident.origin.where}</code>
          {incident.origin.code && <code className="line">{incident.origin.code}</code>}
        </div>
      )}
      {incident.likelyCause && (
        <div className="incident-row">
          <b>Likely cause</b> {incident.likelyCause}
        </div>
      )}
      {incident.suggestion && (
        <div className="incident-row">
          <b>Try</b> {incident.suggestion}
        </div>
      )}
      <div className="incident-tags">
        {incident.services.map((s) => (
          <span key={`s-${s}`} className="chip">{s}</span>
        ))}
        {incident.devices.map((d) => (
          <span key={`d-${d}`} className="chip tone-info">{d}</span>
        ))}
        {incident.scan && <span className="chip">scan {incident.scan}</span>}
        {incident.related.length > 0 && <span className="muted">with {incident.related.join(", ")}</span>}
      </div>
      <div className="incident-actions">
        {actions?.ask && (
          <button
            className="btn small"
            onClick={() =>
              actions.ask!(
                `Explain BEC log incident ${incident.ref} (${incident.title}, first record ${incident.firstId ?? "?"}) and suggest a fix.`,
              )
            }
          >
            Ask about it
          </button>
        )}
        {incident.tracebackRecord !== undefined && actions?.showCode && (
          <button className="btn small" onClick={() => actions.showCode!(incident.tracebackRecord!)}>
            Show code
          </button>
        )}
        {incident.firstId !== undefined && actions?.showRecords && (
          <button className="btn small" onClick={() => actions.showRecords!(incident.firstId!)}>
            Show records
          </button>
        )}
        {incident.context.length > 0 && (
          <button className="btn small ghost" onClick={() => setOpen(!open)}>
            <IconChevron size={12} className="chev" style={{ transform: open ? "rotate(90deg)" : undefined }} />
            {open ? "Hide" : "Records around it"}
          </button>
        )}
      </div>
      {open && <LogRecords records={incident.context} actions={actions} />}
    </div>
  );
}

export function IncidentList({ incidents, actions }: { incidents: IncidentView[]; actions?: LogActions }) {
  if (!incidents.length) return <div className="muted">No incidents.</div>;
  return (
    <div className="incidents">
      {incidents.map((incident) => (
        <Incident key={incident.ref} incident={incident} actions={actions} />
      ))}
    </div>
  );
}

function Frame({ frame, initiallyOpen }: { frame: FrameView; initiallyOpen: boolean }) {
  const [open, setOpen] = useState(initiallyOpen);
  const mismatch = frame.sourceCheck && frame.sourceCheck !== "matches";
  return (
    <div className="frame">
      <button className="frame-head" onClick={() => setOpen(!open)} disabled={!frame.code}>
        <IconChevron size={12} className="chev" style={{ transform: open ? "rotate(90deg)" : undefined, opacity: frame.code ? 1 : 0.3 }} />
        <span className="fn">{frame.function}</span>
        <span className="where">{frame.where}</span>
        {!frame.underRoots && <span className="chip">outside the code roots</span>}
        {mismatch && <span className="chip tone-warn" title={frame.sourceCheck}>source differs</span>}
      </button>
      {open && frame.code && <CodeLines code={frame.code} />}
    </div>
  );
}

export function TracebackView({ frames, exceptions }: { frames: FrameView[]; exceptions: { text: string; relation: string }[] }) {
  return (
    <div className="traceback">
      {exceptions.length > 0 && (
        <div className="exceptions">
          {exceptions.map((e, i) => (
            <div key={i} className="exception">
              {i > 0 && <span className="muted">{e.relation === "context" ? "during handling, " : "which caused "}</span>}
              <code>{e.text}</code>
            </div>
          ))}
        </div>
      )}
      <div className="frames">
        {frames.map((frame, i) => (
          <Frame key={frame.index} frame={frame} initiallyOpen={i === frames.length - 1} />
        ))}
      </div>
    </div>
  );
}
