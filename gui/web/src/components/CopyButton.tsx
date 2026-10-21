import { useState } from "react";
import { IconCheck, IconCopy } from "./Icons";

export function CopyButton({ text, className = "" }: { text: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className={`icon-btn ${className}`}
      title={done ? "Copied" : "Copy"}
      onClick={(event) => {
        event.stopPropagation();
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1200);
        });
      }}
    >
      {done ? <IconCheck size={14} /> : <IconCopy size={14} />}
    </button>
  );
}
