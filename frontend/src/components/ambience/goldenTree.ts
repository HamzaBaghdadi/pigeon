import { glowSprite, offscreen, rand, seeded, type Scene } from "./shared";

/**
 * Gilded: a vast tree of golden light on the horizon, behind the reading pane.
 *
 * The tree is generated (original art, the same every time) and painted once
 * to an offscreen canvas together with a cheap blurred "bloom" copy. Each
 * frame then only stamps those images and animates around them: the crown's
 * halo breathes, rays turn slowly, a ring of glyphs rotates behind the crown,
 * leaves of light fall from the branches and embers rise from the ground.
 */
export function gildedScene(w: number, h: number, dark: boolean): Scene {
  const gold = dark ? "255,212,128" : "150,104,28";
  const pale = dark ? "255,238,196" : "112,76,18";
  const additive: GlobalCompositeOperation = dark ? "lighter" : "source-over";
  const strength = dark ? 1 : 0.6;

  // Tree placement: centered behind the reader pane, crown near the top.
  const size = Math.min(w, h * 1.4);
  const baseX = w * 0.66;
  const baseY = h * 1.03;
  const forkY = h * 0.5;
  const crown = { x: baseX, y: h * 0.26 };

  const tree = paintTree(w, h, baseX, baseY, forkY, size, gold, pale, additive);
  // Bloom: the tree drawn small and scaled back up is a fast, soft blur.
  const bloomScale = 0.18;
  const bloom = offscreen(w * bloomScale, h * bloomScale);
  bloom.g.drawImage(tree.canvas, 0, 0, w * bloomScale, h * bloomScale);

  const ring = paintGlyphRing(size * 0.34, gold);
  const leafGlow = glowSprite(gold);
  const leafCore = glowSprite(pale, 24);

  // Leaves fall from the branch tips; embers rise from the horizon.
  const tips = tree.tips;
  const newLeaf = (anywhere: boolean) => {
    const tip = tips[Math.floor(Math.random() * tips.length)] ?? crown;
    return {
      x: tip.x + rand(-10, 10),
      y: anywhere ? rand(tip.y, h) : tip.y + rand(-6, 6),
      r: rand(0.8, 2.2),
      vy: rand(0.12, 0.38),
      vx: rand(-0.12, 0.05),
      sway: rand(6, 22),
      freq: rand(0.4, 1.1),
      phase: rand(0, Math.PI * 2),
      life: rand(0.5, 1),
    };
  };
  const leaves = Array.from(
    { length: Math.round(70 * clamp((w * h) / (1280 * 800), 0.5, 1.6)) },
    () => newLeaf(true),
  );
  const newEmber = (anywhere: boolean) => ({
    x: rand(0, w),
    y: anywhere ? rand(h * 0.4, h) : h + rand(5, 40),
    r: rand(0.6, 1.6),
    vy: rand(0.2, 0.5),
    phase: rand(0, Math.PI * 2),
  });
  const embers = Array.from({ length: 26 }, () => newEmber(true));

  return {
    step(ctx, dt, t) {
      const breath = 0.5 + 0.5 * Math.sin(t * 0.55); // slow pulse, 0..1
      ctx.globalCompositeOperation = additive;

      // Great halo around the crown.
      const haloR = size * 0.75;
      const halo = ctx.createRadialGradient(
        crown.x,
        crown.y,
        0,
        crown.x,
        crown.y,
        haloR,
      );
      halo.addColorStop(0, `rgba(${gold},${(0.2 + 0.08 * breath) * strength})`);
      halo.addColorStop(0.35, `rgba(${gold},${0.07 * strength})`);
      halo.addColorStop(1, `rgba(${gold},0)`);
      ctx.fillStyle = halo;
      ctx.fillRect(0, 0, w, h);

      // Rays turning slowly around the crown.
      ctx.save();
      ctx.translate(crown.x, crown.y);
      ctx.rotate(t * 0.015);
      const rayLen = Math.hypot(w, h);
      for (let i = 0; i < 14; i++) {
        const a = (i / 14) * Math.PI * 2;
        const wdt = i % 2 ? 0.035 : 0.06;
        const grad = ctx.createRadialGradient(
          0,
          0,
          size * 0.05,
          0,
          0,
          rayLen * 0.6,
        );
        grad.addColorStop(
          0,
          `rgba(${gold},${(0.06 + 0.03 * breath) * strength})`,
        );
        grad.addColorStop(1, `rgba(${gold},0)`);
        ctx.fillStyle = grad;
        ctx.beginPath();
        ctx.moveTo(0, 0);
        ctx.lineTo(Math.cos(a - wdt) * rayLen, Math.sin(a - wdt) * rayLen);
        ctx.lineTo(Math.cos(a + wdt) * rayLen, Math.sin(a + wdt) * rayLen);
        ctx.closePath();
        ctx.fill();
      }
      ctx.restore();

      // Ring of glyphs behind the crown.
      ctx.save();
      ctx.translate(crown.x, crown.y);
      ctx.rotate(-t * 0.02);
      ctx.globalAlpha = (0.32 + 0.12 * breath) * strength;
      ctx.drawImage(ring, -ring.width / 2, -ring.height / 2);
      ctx.restore();

      // Horizon glow where the roots meet the ground.
      const ground = ctx.createRadialGradient(
        baseX,
        h,
        0,
        baseX,
        h,
        size * 0.6,
      );
      ground.addColorStop(0, `rgba(${gold},${0.22 * strength})`);
      ground.addColorStop(1, `rgba(${gold},0)`);
      ctx.fillStyle = ground;
      ctx.fillRect(0, h * 0.5, w, h * 0.5);

      // The tree, with its bloom breathing.
      ctx.globalAlpha = (0.55 + 0.35 * breath) * strength;
      ctx.drawImage(bloom.canvas, 0, 0, w, h);
      ctx.globalAlpha = (0.85 + 0.15 * breath) * (dark ? 1 : 0.75);
      ctx.drawImage(tree.canvas, 0, 0);

      // Falling leaves of light.
      for (let i = 0; i < leaves.length; i++) {
        const l = leaves[i];
        l.y += l.vy * dt;
        l.x += l.vx * dt;
        if (l.y > h + 10) {
          leaves[i] = newLeaf(false);
          continue;
        }
        const x = l.x + Math.sin(t * l.freq + l.phase) * l.sway;
        const fade = 1 - Math.max(0, (l.y - h * 0.7) / (h * 0.35)); // fade near the ground
        const a =
          (0.6 + 0.4 * Math.sin(t * 2.2 + l.phase)) * l.life * fade * strength;
        const s = l.r * 10;
        ctx.globalAlpha = a * 0.6;
        ctx.drawImage(leafGlow, x - s / 2, l.y - s / 2, s, s);
        ctx.globalAlpha = a;
        ctx.drawImage(
          leafCore,
          x - l.r * 1.5,
          l.y - l.r * 1.5,
          l.r * 3,
          l.r * 3,
        );
      }

      // Embers rising from the horizon.
      for (let i = 0; i < embers.length; i++) {
        const e = embers[i];
        e.y -= e.vy * dt;
        if (e.y < h * 0.3) {
          embers[i] = newEmber(false);
          continue;
        }
        const x = e.x + Math.sin(t * 0.7 + e.phase) * 12;
        const a =
          (0.5 + 0.5 * Math.sin(t * 3 + e.phase)) *
          ((e.y - h * 0.3) / (h * 0.7)) *
          strength;
        const s = e.r * 9;
        ctx.globalAlpha = a * 0.7;
        ctx.drawImage(leafGlow, x - s / 2, e.y - s / 2, s, s);
      }

      ctx.globalAlpha = 1;
      ctx.globalCompositeOperation = "source-over";
    },
  };
}

const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));

type Point = { x: number; y: number };

/**
 * Paints the tree: a trunk of twisted golden strands that parts into great
 * limbs, which branch again and again into a wide, luminous crown.
 */
function paintTree(
  w: number,
  h: number,
  baseX: number,
  baseY: number,
  forkY: number,
  size: number,
  gold: string,
  pale: string,
  mode: GlobalCompositeOperation,
) {
  const { canvas, g } = offscreen(w, h);
  const rnd = seeded(1729);
  const tips: Point[] = [];
  g.lineCap = "round";
  g.globalCompositeOperation = mode;
  g.shadowColor = `rgba(${gold},0.9)`;

  // Trunk: strands that wind around each other on the way up.
  const strands = 9;
  const trunk = size * 0.055;
  for (let i = 0; i < strands; i++) {
    const o = (i / (strands - 1) - 0.5) * trunk;
    const twist = (rnd() - 0.5) * trunk * 1.4;
    g.strokeStyle = `rgba(${i % 3 === 0 ? pale : gold},${0.45 + rnd() * 0.25})`;
    g.lineWidth = 1.6 + rnd() * 2.2;
    g.shadowBlur = 16;
    g.beginPath();
    g.moveTo(baseX + o * 2.4, baseY);
    g.bezierCurveTo(
      baseX - o * 0.6 + twist,
      baseY - (baseY - forkY) * 0.35,
      baseX + o * 0.9 - twist,
      baseY - (baseY - forkY) * 0.7,
      baseX + o * 0.35,
      forkY,
    );
    g.stroke();
  }
  // Roots spreading along the horizon.
  for (let i = 0; i < 8; i++) {
    const dir = i % 2 ? 1 : -1;
    const len = size * (0.12 + rnd() * 0.18);
    g.strokeStyle = `rgba(${gold},${0.25 + rnd() * 0.2})`;
    g.lineWidth = 1 + rnd() * 1.6;
    g.shadowBlur = 10;
    g.beginPath();
    g.moveTo(baseX + dir * trunk * rnd(), baseY - 4);
    g.quadraticCurveTo(
      baseX + dir * len * 0.5,
      baseY - 12 - rnd() * 18,
      baseX + dir * len,
      baseY + 2,
    );
    g.stroke();
  }

  const branch = (
    x: number,
    y: number,
    angle: number,
    len: number,
    width: number,
    depth: number,
  ) => {
    const bend = (rnd() - 0.5) * 0.6;
    const x2 = x + Math.sin(angle) * len;
    const y2 = y - Math.cos(angle) * len;
    const cx = x + Math.sin(angle + bend) * len * 0.55;
    const cy = y - Math.cos(angle + bend) * len * 0.55;
    g.strokeStyle = `rgba(${depth > 4 ? pale : gold},${depth > 3 ? 0.3 + depth * 0.07 : 0.1 + depth * 0.05})`;
    g.lineWidth = width;
    g.shadowBlur = 6 + depth * 2;
    g.beginPath();
    g.moveTo(x, y);
    g.quadraticCurveTo(cx, cy, x2, y2);
    g.stroke();
    if (depth <= 1 || len < 5) {
      tips.push({ x: x2, y: y2 });
      return;
    }
    const kids = depth > 5 ? 3 : 2;
    for (let k = 0; k < kids; k++) {
      const spread = 0.32 + rnd() * 0.3;
      // Branches lean outward and droop a little toward the crown's edge.
      const next =
        angle +
        (k - (kids - 1) / 2) * spread * 1.3 +
        (rnd() - 0.5) * 0.25 +
        angle * 0.08;
      branch(
        x2,
        y2,
        next,
        len * (0.7 + rnd() * 0.1),
        Math.max(0.5, width * 0.66),
        depth - 1,
      );
    }
  };

  // Great limbs fanning out from the fork into a wide dome.
  const limbs = [-1.25, -0.85, -0.45, -0.12, 0.12, 0.45, 0.85, 1.25];
  for (const a of limbs) {
    const len = size * (0.13 + rnd() * 0.04) * (1 - Math.abs(a) * 0.12);
    branch(
      baseX + a * trunk * 0.3,
      forkY,
      a + (rnd() - 0.5) * 0.1,
      len,
      3.2,
      7,
    );
  }

  // Glowing buds at the tips make the crown read as light, not wood.
  g.shadowBlur = 0;
  const bud = glowSprite(pale, 32);
  for (const p of tips) {
    const s = 6 + rnd() * 10;
    g.globalAlpha = 0.35 + rnd() * 0.4;
    g.drawImage(bud, p.x - s / 2, p.y - s / 2, s, s);
  }
  g.globalAlpha = 1;
  return { canvas, tips };
}

/** A thin double ring with abstract glyph marks around it (original, not text). */
function paintGlyphRing(radius: number, gold: string) {
  const pad = 20;
  const d = Math.ceil(radius * 2 + pad * 2);
  const { canvas, g } = offscreen(d, d);
  const rnd = seeded(42);
  const c = d / 2;
  g.strokeStyle = `rgba(${gold},0.9)`;
  g.lineCap = "round";
  g.shadowColor = `rgba(${gold},0.8)`;
  g.shadowBlur = 6;

  g.lineWidth = 1;
  g.beginPath();
  g.arc(c, c, radius, 0, Math.PI * 2);
  g.stroke();
  g.lineWidth = 0.7;
  g.beginPath();
  g.arc(c, c, radius - 14, 0, Math.PI * 2);
  g.stroke();

  // Glyphs: each is two or three short strokes, set between the two rings.
  const count = 48;
  for (let i = 0; i < count; i++) {
    const a = (i / count) * Math.PI * 2;
    g.save();
    g.translate(c + Math.cos(a) * (radius - 7), c + Math.sin(a) * (radius - 7));
    g.rotate(a + Math.PI / 2);
    g.lineWidth = 0.9;
    const strokes = 2 + Math.floor(rnd() * 2);
    g.beginPath();
    for (let s = 0; s < strokes; s++) {
      const x1 = (rnd() - 0.5) * 6;
      const y1 = (rnd() - 0.5) * 8;
      g.moveTo(x1, y1);
      g.lineTo(x1 + (rnd() - 0.5) * 7, y1 + (rnd() - 0.5) * 8);
    }
    g.stroke();
    g.restore();
  }
  // Small diamonds at the four quarters.
  for (let q = 0; q < 4; q++) {
    const a = (q / 4) * Math.PI * 2;
    g.save();
    g.translate(c + Math.cos(a) * radius, c + Math.sin(a) * radius);
    g.rotate(a);
    g.beginPath();
    g.moveTo(0, -5);
    g.lineTo(4, 0);
    g.lineTo(0, 5);
    g.lineTo(-4, 0);
    g.closePath();
    g.fillStyle = `rgba(${gold},0.9)`;
    g.fill();
    g.restore();
  }
  return canvas;
}
