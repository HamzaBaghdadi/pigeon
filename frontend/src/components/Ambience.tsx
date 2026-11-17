"use client";

import { useEffect, useRef, useState } from "react";
import { blossomScene } from "./ambience/blossom";
import { gildedScene } from "./ambience/goldenTree";
import type { Scene } from "./ambience/shared";

type Style = "blossom" | "gilded" | null;

/**
 * The animated background behind the panes for themes that have one:
 * a golden tree of light for Gilded, falling petals and sparkles
 * for Blossom. Purely decorative: hidden from assistive tech, never
 * interactive, paused while the window is hidden, and drawn once (no motion)
 * when Windows asks for reduced motion.
 */
export default function Ambience() {
  const style = useHtmlStyle();
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas || !style) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)");
    let w = 0;
    let h = 0;
    let scene: Scene;
    let raf = 0;
    let last = 0;

    const build = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
      w = window.innerWidth;
      h = window.innerHeight;
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      scene =
        style === "gilded"
          ? gildedScene(w, h, isDark())
          : blossomScene(w, h, isDark());
    };

    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      if (now - last < 33) return; // ~30 fps is plenty for slow ambience
      const dt = last ? Math.min((now - last) / 16.7, 3) : 1;
      last = now;
      ctx.clearRect(0, 0, w, h);
      scene.step(ctx, dt, now / 1000);
    };

    const start = () => {
      cancelAnimationFrame(raf);
      last = 0;
      if (reduce.matches) {
        ctx.clearRect(0, 0, w, h);
        scene.step(ctx, 0, 0); // a single still frame
      } else if (!document.hidden) {
        raf = requestAnimationFrame(frame);
      }
    };

    build();
    start();

    const onResize = () => {
      build();
      start();
    };
    // Generated art is redrawn once the window stops resizing.
    let resizeTimer = 0;
    const onResizeDebounced = () => {
      clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(onResize, 150);
    };
    const onVisibility = () =>
      document.hidden ? cancelAnimationFrame(raf) : start();
    // Rebuild when light/dark flips (forced or from Windows).
    const scheme = window.matchMedia("(prefers-color-scheme: dark)");
    const observer = new MutationObserver(onResize);
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });

    window.addEventListener("resize", onResizeDebounced);
    document.addEventListener("visibilitychange", onVisibility);
    scheme.addEventListener("change", onResize);
    reduce.addEventListener("change", start);
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      window.removeEventListener("resize", onResizeDebounced);
      clearTimeout(resizeTimer);
      document.removeEventListener("visibilitychange", onVisibility);
      scheme.removeEventListener("change", onResize);
      reduce.removeEventListener("change", start);
    };
  }, [style]);

  if (!style) return null;
  return (
    <canvas
      ref={ref}
      aria-hidden
      className="pointer-events-none fixed inset-0 z-0 h-full w-full"
    />
  );
}

/** Tracks data-style on <html>, which the theme setting controls. */
function useHtmlStyle(): Style {
  const read = (): Style => {
    const s = document.documentElement.dataset.style;
    return s === "blossom" || s === "gilded" ? s : null;
  };
  const [style, setStyle] = useState<Style>(read);
  useEffect(() => {
    const observer = new MutationObserver(() => setStyle(read()));
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-style"],
    });
    return () => observer.disconnect();
  }, []);
  return style;
}

function isDark() {
  const t = document.documentElement.dataset.theme;
  if (t === "dark") return true;
  if (t === "light") return false;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}
