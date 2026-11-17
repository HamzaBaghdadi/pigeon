/** One animated background. step() draws a frame; dt is in 60-fps frames. */
export type Scene = {
  step: (ctx: CanvasRenderingContext2D, dt: number, t: number) => void;
};

export const rand = (a: number, b: number) => a + Math.random() * (b - a);

/** A small seeded random generator, so generated art looks the same every time. */
export function seeded(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function offscreen(w: number, h: number) {
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(w));
  c.height = Math.max(1, Math.round(h));
  return { canvas: c, g: c.getContext("2d")! };
}

/** A soft round glow, pre-rendered once and stamped for every particle. */
export function glowSprite(rgb: string, size = 64) {
  const { canvas, g } = offscreen(size, size);
  const grad = g.createRadialGradient(
    size / 2,
    size / 2,
    0,
    size / 2,
    size / 2,
    size / 2,
  );
  grad.addColorStop(0, `rgba(${rgb},1)`);
  grad.addColorStop(0.25, `rgba(${rgb},0.55)`);
  grad.addColorStop(1, `rgba(${rgb},0)`);
  g.fillStyle = grad;
  g.fillRect(0, 0, size, size);
  return canvas;
}
