"use client";

import { useCallback, useRef } from "react";
import { Tip } from "./Tip";

type Props = {
  /** Accessible name, e.g. "Resize sidebar". */
  label: string;
  value: number;
  min: number;
  max: number;
  defaultValue: number;
  onChange: (width: number) => void;
};

/**
 * Vertical pane divider. Drag to resize, arrow keys for keyboard users
 * (Shift for bigger steps), Home/End for min/max, double-click to reset.
 */
export default function Splitter({ label, value, min, max, defaultValue, onChange }: Props) {
  const start = useRef<{ x: number; w: number } | null>(null);
  const clamp = useCallback((w: number) => Math.round(Math.min(max, Math.max(min, w))), [min, max]);

  return (
    <Tip label="Drag to resize · double-click to reset" side="right">
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={value}
      aria-valuemin={min}
      aria-valuemax={max}
      tabIndex={0}
      onPointerDown={(e) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        start.current = { x: e.clientX, w: value };
        document.body.style.cursor = "col-resize";
      }}
      onPointerMove={(e) => {
        if (start.current) onChange(clamp(start.current.w + e.clientX - start.current.x));
      }}
      onPointerUp={(e) => {
        e.currentTarget.releasePointerCapture(e.pointerId);
        start.current = null;
        document.body.style.cursor = "";
      }}
      onDoubleClick={() => onChange(defaultValue)}
      onKeyDown={(e) => {
        const step = e.shiftKey ? 48 : 16;
        if (e.key === "ArrowLeft") onChange(clamp(value - step));
        else if (e.key === "ArrowRight") onChange(clamp(value + step));
        else if (e.key === "Home") onChange(min);
        else if (e.key === "End") onChange(max);
        else return;
        e.preventDefault();
        e.stopPropagation();
      }}
      className="group relative z-10 -mx-[3px] w-[7px] shrink-0 cursor-col-resize outline-none"
    >
      <div className="mx-auto h-full w-px bg-border transition-colors duration-150 group-hover:bg-primary group-focus-visible:bg-primary group-active:bg-primary" />
    </div>
    </Tip>
  );
}
