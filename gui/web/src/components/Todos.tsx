import { useState } from "react";
import { useStore } from "../state/store";
import { IconCheck, IconChevron, IconList } from "./Icons";

const DONE = new Set(["completed", "cancelled"]);

/** The agent's todo list for the open chat, above the composer while work is left. */
export function Todos() {
  const todos = useStore((s) => (s.currentId ? s.todos[s.currentId] : undefined));
  const [open, setOpen] = useState(true);
  if (!todos?.length) return null;
  const done = todos.filter((t) => DONE.has(t.status)).length;
  if (done === todos.length) return null;
  const current = todos.find((t) => t.status === "in_progress");
  return (
    <div className="requests">
      <div className="todos">
        <button className="todos-head" onClick={() => setOpen(!open)}>
          <IconChevron size={12} className="chev" style={{ transform: open ? "rotate(90deg)" : undefined }} />
          <IconList size={14} />
          <span className="grow">{current ? current.content : "Plan"}</span>
          <span className="muted">
            {done}/{todos.length} done
          </span>
        </button>
        {open && (
          <ul>
            {todos.map((todo, i) => (
              <li key={i} className={`todo ${todo.status}`}>
                <span className="mark">
                  {todo.status === "completed" ? (
                    <IconCheck size={12} />
                  ) : todo.status === "in_progress" ? (
                    <span className="spinner" style={{ width: 10, height: 10, borderWidth: 1.5 }} />
                  ) : (
                    <span className="dot" />
                  )}
                </span>
                <span>{todo.content}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
