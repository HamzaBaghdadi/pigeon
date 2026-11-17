import { glowSprite, rand, type Scene } from "./shared";

// ---- Blossom: petals drifting down, sparkles twinkling ----

export function blossomScene(w: number, h: number, dark: boolean): Scene {
  const palette = dark
    ? ["255,150,200", "255,184,220", "214,170,255", "255,200,180"]
    : ["255,150,195", "255,182,213", "212,176,255", "255,196,170"];
  const sparkle = glowSprite(dark ? "255,230,245" : "255,255,255", 48);
  const density = (w * h) / (1280 * 800);
  const petalCount = Math.round(26 * Math.min(Math.max(density, 0.5), 1.6));

  const newPetal = (anywhere: boolean) => ({
    x: rand(-40, w),
    y: anywhere ? rand(0, h) : rand(-60, -20),
    size: rand(7, 14),
    vx: rand(0.15, 0.45),
    vy: rand(0.25, 0.6),
    rot: rand(0, Math.PI * 2),
    vrot: rand(-0.02, 0.02),
    flip: rand(0, Math.PI * 2),
    vflip: rand(0.02, 0.05),
    color: palette[Math.floor(rand(0, palette.length))],
    alpha: rand(0.6, 0.95),
  });
  const petals = Array.from({ length: petalCount }, () => newPetal(true));
  const sparkles = Array.from({ length: 16 }, () => ({
    x: rand(0, w),
    y: rand(0, h),
    r: rand(1.5, 3.5),
    speed: rand(0.6, 1.6),
    phase: rand(0, Math.PI * 2),
  }));

  const drawPetal = (ctx: CanvasRenderingContext2D, s: number) => {
    ctx.beginPath();
    ctx.moveTo(0, -s);
    ctx.bezierCurveTo(s * 0.9, -s * 0.6, s * 0.7, s * 0.6, 0, s);
    ctx.bezierCurveTo(-s * 0.7, s * 0.6, -s * 0.9, -s * 0.6, 0, -s);
    ctx.fill();
  };

  return {
    step(ctx, dt, t) {
      for (const sp of sparkles) {
        const a = Math.max(0, Math.sin(t * sp.speed + sp.phase));
        if (a < 0.05) continue;
        const s = sp.r * 6 * (0.6 + a * 0.4);
        ctx.globalAlpha = a * (dark ? 0.7 : 0.9);
        ctx.drawImage(sparkle, sp.x - s / 2, sp.y - s / 2, s, s);
        // A tiny four-point star in the middle.
        ctx.fillStyle = dark
          ? "rgba(255,240,250,0.9)"
          : "rgba(255,255,255,0.95)";
        ctx.beginPath();
        const k = sp.r * (0.8 + a);
        ctx.moveTo(sp.x, sp.y - k);
        ctx.lineTo(sp.x + k * 0.22, sp.y);
        ctx.lineTo(sp.x, sp.y + k);
        ctx.lineTo(sp.x - k * 0.22, sp.y);
        ctx.closePath();
        ctx.moveTo(sp.x - k, sp.y);
        ctx.lineTo(sp.x, sp.y + k * 0.22);
        ctx.lineTo(sp.x + k, sp.y);
        ctx.lineTo(sp.x, sp.y - k * 0.22);
        ctx.closePath();
        ctx.fill();
      }
      for (let i = 0; i < petals.length; i++) {
        const p = petals[i];
        p.x += (p.vx + Math.sin(t * 0.8 + p.flip) * 0.25) * dt;
        p.y += p.vy * dt;
        p.rot += p.vrot * dt;
        p.flip += p.vflip * dt;
        if (p.y > h + 30 || p.x > w + 40) {
          petals[i] = newPetal(false);
          continue;
        }
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rot);
        ctx.scale(Math.cos(p.flip) * 0.8 + 0.2, 1); // tumbling
        ctx.globalAlpha = p.alpha * (dark ? 0.85 : 0.95);
        ctx.fillStyle = `rgba(${p.color},1)`;
        drawPetal(ctx, p.size);
        ctx.restore();
      }
      ctx.globalAlpha = 1;
    },
  };
}
