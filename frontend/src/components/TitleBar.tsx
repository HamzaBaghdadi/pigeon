"use client";

import { useEffect, useState } from "react";
import { Window } from "@wailsio/runtime";
import { Copy, Minus, Square, X } from "lucide-react";
import { Tip } from "./Tip";

/**
 * Pigeon's own title bar (the window is frameless). The bar drags the window
 * and double-clicking it maximizes; the controls mirror Windows 11's.
 */
export default function TitleBar() {
  const [maximised, setMaximised] = useState(false);

  useEffect(() => {
    let alive = true;
    const check = () => {
      Window.IsMaximised()
        .then((m) => alive && setMaximised(m))
        .catch(() => {});
    };
    check();
    window.addEventListener("resize", check);
    return () => {
      alive = false;
      window.removeEventListener("resize", check);
    };
  }, []);

  return (
    <header
      className="flex h-9 shrink-0 items-center border-b border-border bg-pane-side select-none titlebar"
      style={{ ["--wails-draggable" as string]: "drag" }}
      onDoubleClick={(e) => {
        if (e.target === e.currentTarget || (e.target as HTMLElement).dataset.drag === "true") Window.ToggleMaximise().catch(() => {});
      }}
    >
      <div className="flex items-center gap-2 pl-3.5" data-drag="true">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/pigeon.png" alt="" className="pointer-events-none h-4 w-4" draggable={false} />
        <span className="brand-word pointer-events-none font-display text-[12.5px] font-semibold tracking-[-0.01em]">Pigeon</span>
      </div>
      <div className="h-full flex-1" data-drag="true" />
      <div className="flex h-full" style={{ ["--wails-draggable" as string]: "no-drag" }}>
        <Tip label="Minimize">
          <button
            onClick={() => Window.Minimise()}
            aria-label="Minimize"
            className="flex h-full w-[46px] items-center justify-center text-muted transition-colors duration-100 hover:bg-hover hover:text-text"
          >
            <Minus size={15} strokeWidth={1.5} />
          </button>
        </Tip>
        <Tip label={maximised ? "Restore" : "Maximize"}>
          <button
            onClick={() => Window.ToggleMaximise().then(() => setTimeout(() => Window.IsMaximised().then(setMaximised), 50))}
            aria-label={maximised ? "Restore" : "Maximize"}
            className="flex h-full w-[46px] items-center justify-center text-muted transition-colors duration-100 hover:bg-hover hover:text-text"
          >
            {maximised ? <Copy size={12} strokeWidth={1.5} className="-scale-x-100" /> : <Square size={11.5} strokeWidth={1.5} />}
          </button>
        </Tip>
        <Tip label="Close">
          <button
            onClick={() => Window.Close()}
            aria-label="Close"
            className="flex h-full w-[46px] items-center justify-center text-muted transition-colors duration-100 hover:bg-[#c42b1c] hover:text-white"
          >
            <X size={16} strokeWidth={1.5} />
          </button>
        </Tip>
      </div>
    </header>
  );
}
