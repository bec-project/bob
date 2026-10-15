import { CopyButton } from "../../components/CopyButton";
import { openExternal } from "../../platform";
import type { Card } from "./cards";
import { CodeLines, IncidentList, LogRecords, TracebackView, type LogActions } from "./LogViews";

function toneClass(tone?: string) {
  return tone && tone !== "neutral" ? `tone-${tone}` : "";
}

/** Draws one BEC report card; sections without content are left out. */
export function ReportCard({
  card,
  compact = false,
  actions,
}: {
  card: Card;
  compact?: boolean;
  /** Buttons on findings, incidents and log records; left out where nothing can act on them. */
  actions?: LogActions;
}) {
  return (
    <div className="card">
      {(!compact || card.badges.length > 0) && (
        <div className="card-head">
          {!compact && <span className="card-title">{card.title}</span>}
          {card.badges.map((badge, i) => (
            <span key={i} className={`chip ${toneClass(badge.tone)}`}>
              {badge.text}
            </span>
          ))}
        </div>
      )}
      {!compact && card.summary && <div className="card-summary">{card.summary}</div>}

      {card.gauge && (
        <div className="gauge">
          <div className="top">
            <span className="value">{card.gauge.value}</span>
            <span className="muted">{card.gauge.label}</span>
            {card.gauge.age && <span className="muted" style={{ marginLeft: "auto", fontSize: 12 }}>{card.gauge.age}</span>}
          </div>
          {card.gauge.fraction >= 0 && (
            <>
              <div className="bar">
                <div
                  className={`marker ${card.gauge.tone === "warn" ? "warn" : ""}`}
                  style={{ left: `${card.gauge.fraction * 100}%` }}
                />
              </div>
              <div className="limits">
                <span>{card.gauge.low}</span>
                <span>limits</span>
                <span>{card.gauge.high}</span>
              </div>
            </>
          )}
        </div>
      )}

      {card.stats.length > 0 && (
        <div className="card-stats">
          {card.stats.map((stat, i) => (
            <div key={i} className="stat">
              <div className={`v ${toneClass(stat.tone)}`}>{stat.value}</div>
              <div className="l">{stat.label}</div>
            </div>
          ))}
        </div>
      )}

      {card.findings.length > 0 && (
        <div className="findings">
          {card.findings.map((finding, i) => (
            <div key={i} className={`finding ${toneClass(finding.tone)}`}>
              <div className="body">{finding.text}</div>
              {finding.next && (
                <div className="next">
                  Next: <code>{finding.next}</code>
                  {actions?.ask && (
                    <button className="btn small ghost" onClick={() => actions.ask!(`Run ${finding.next} and tell me what it shows.`)}>
                      Ask
                    </button>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {card.incidents && <IncidentList incidents={card.incidents} actions={actions} />}
      {card.records && <LogRecords records={card.records} actions={actions} />}
      {card.frames && <TracebackView frames={card.frames} exceptions={card.exceptions ?? []} />}

      {card.table && card.table.rows.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                {card.table.columns.map((column, i) => (
                  <th key={i}>{column}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {card.table.rows.map((row, r) => (
                <tr key={r}>
                  {row.map((value, c) => (
                    <td key={c} className={toneClass(value.tone)}>
                      {value.text}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {card.table.note && <div className="table-note">{card.table.note}</div>}
        </div>
      )}

      {card.lines.length > 0 && (
        <div className="log-lines">
          {card.lines.map((line, i) => (
            <div key={i} className="log-line">
              <div className={toneClass(line.tone)}>{line.text}</div>
              <div className="meta">{line.meta}</div>
            </div>
          ))}
        </div>
      )}

      {card.code && (
        <div className="code-box">
          <div className="label">{card.code.label}</div>
          {/^\s*>?\s*\d+\|/.test(card.code.text) ? (
            <CodeLines code={card.code.text} />
          ) : (
            <pre>
              <code>{card.code.text}</code>
            </pre>
          )}
          <CopyButton text={card.code.text} className="copy" />
        </div>
      )}

      {card.links.map((link, i) => (
        <a
          key={i}
          className="doc-link"
          href={link.url || undefined}
          onClick={(event) => {
            if (!link.url) return;
            event.preventDefault();
            void openExternal(link.url);
          }}
        >
          <div className="t">{link.title}</div>
          {link.subtitle && <div className="s">{link.subtitle}</div>}
          {link.snippet && <div className="muted" style={{ fontSize: 12.5 }}>{link.snippet}</div>}
        </a>
      ))}
    </div>
  );
}
