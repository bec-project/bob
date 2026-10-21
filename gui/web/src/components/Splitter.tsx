import { useRef, useState } from "react";
import { useStore } from "../state/store";

/** Keeps a resized width inside the window: at least min, and leaving room for the chat. */
export function clampWidth(width: number, min: number, windowWidth: number): number {
  return Math.round(Math.min(Math.max(width, min), Math.max(min, windowWidth - 480)));
}

/**
 * A width the user sets by dragging a splitter, remembered under `key` (in the page's prefs).
 * Returns the width to draw with and the splitter to put on the element's edge: "left" for an
 * element docked on the right (dragging left widens it), "right" for one docked on the left.
 */
export function useResizable(key: string, initial: number, side: "left" | "right", min = 240) {
  const stored = useStore((s) => s.prefs.widths[key]);
  const setPrefs = useStore((s) => s.setPrefs);
  const [dragging, setDragging] = useState<number>();
  const start = useRef<{ x: number; width: number; last: number }>(undefined);
  const width = dragging ?? stored ?? initial;

  const save = (value: number | undefined) => {
    const widths = { ...useStore.getState().prefs.widths };
    if (value === undefined) delete widths[key];
    else widths[key] = value;
    setPrefs({ widths });
  };

  const splitter = (
    <div
      className={`splitter ${side}`}
      role="separator"
      aria-orientation="vertical"
      title="Drag to resize; double-click for the default width"
      onPointerDown={(event) => {
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        start.current = { x: event.clientX, width, last: width };
        document.body.classList.add("resizing");
      }}
      onPointerMove={(event) => {
        if (!start.current) return;
        const delta = event.clientX - start.current.x;
        const next = clampWidth(start.current.width + (side === "left" ? -delta : delta), min, window.innerWidth);
        start.current.last = next;
        setDragging(next);
      }}
      onPointerUp={(event) => {
        if (!start.current) return;
        event.currentTarget.releasePointerCapture(event.pointerId);
        document.body.classList.remove("resizing");
        save(start.current.last);
        start.current = undefined;
        setDragging(undefined);
      }}
      onDoubleClick={() => save(undefined)}
    />
  );
  return { width, splitter };
}
