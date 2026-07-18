// Generates the AeroShutter source art (SVG + PNG) for @capacitor/assets.
// Renders a multi-blade camera aperture/iris in a warm amber gradient on a
// near-black darkroom background. Rasterization is done with `sharp`
// (already a dependency of @capacitor/assets) so no ImageMagick is required.
//
// Outputs into resources/:
//   icon.svg, icon.png (1024, full-bleed)
//   icon-foreground.png (aperture with ~18% safe margin, transparent)
//   icon-background.png (darkroom background, full-bleed)
//   splash.png, splash-dark.png (2732, centered small logo)
//
// Run: node resources/generate-source.mjs

import sharp from 'sharp';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { writeFileSync } from 'node:fs';

const DIR = dirname(fileURLToPath(import.meta.url));

// ----- palette -----
const BG_EDGE = '#0a0a0b';
const BG_CENTER = '#14110d';

// ----- helpers -----
const rot = (x, y, deg) => {
  const a = (deg * Math.PI) / 180;
  const c = Math.cos(a);
  const s = Math.sin(a);
  return [x * c - y * s, x * s + y * c];
};

// Build an N-blade iris aperture centered at (0,0).
// R = outer radius of the blade ring, r = radius of the (polygonal) opening.
// Each blade is a curved leaf: it sweeps along the outer ring for one segment,
// then its trailing edge cuts straight across to form one side of the opening
// polygon. The leaves overlap so the closed silhouette reads as a filled ring
// with a clean N-gon hole in the middle.
function apertureBlades(N, R, r) {
  const step = 360 / N;
  const blades = [];
  for (let i = 0; i < N; i++) {
    const base = i * step;
    // outer arc endpoints
    const [ox1, oy1] = rot(0, -R, base);
    const [ox2, oy2] = rot(0, -R, base + step);
    // inner opening vertices (the leading edge tucks toward center)
    const [ix2, iy2] = rot(0, -r, base + step);
    // control-ish tuck point that gives the leaf a curved throat
    const [tx, ty] = rot(r * 0.62, -r * 0.35, base + step);
    const d =
      `M ${ox1.toFixed(2)} ${oy1.toFixed(2)} ` +
      `A ${R} ${R} 0 0 1 ${ox2.toFixed(2)} ${oy2.toFixed(2)} ` +
      `L ${ix2.toFixed(2)} ${iy2.toFixed(2)} ` +
      `Q ${tx.toFixed(2)} ${ty.toFixed(2)} ${ox1.toFixed(2)} ${oy1.toFixed(2)} ` +
      `Z`;
    blades.push(d);
  }
  return blades;
}

function defs() {
  return `
    <radialGradient id="bg" cx="50%" cy="42%" r="75%">
      <stop offset="0%" stop-color="${BG_CENTER}"/>
      <stop offset="55%" stop-color="#0d0b0a"/>
      <stop offset="100%" stop-color="${BG_EDGE}"/>
    </radialGradient>
    <linearGradient id="blade" x1="10%" y1="0%" x2="90%" y2="100%">
      <stop offset="0%" stop-color="#ffd178"/>
      <stop offset="40%" stop-color="#ffab2e"/>
      <stop offset="100%" stop-color="#a8490a"/>
    </linearGradient>
    <radialGradient id="glow" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="#ffdd92" stop-opacity="0.5"/>
      <stop offset="45%" stop-color="#ff9d24" stop-opacity="0.16"/>
      <stop offset="100%" stop-color="#ff9d24" stop-opacity="0"/>
    </radialGradient>`;
}

// Draw the aperture mark. `cx,cy` center, `R` outer radius. Returns SVG group.
function apertureMark(cx, cy, R) {
  const N = 6;
  const r = R * 0.30; // opening radius
  const blades = apertureBlades(N, R, r);
  const bladePaths = blades
    .map(
      (d) =>
        `<path d="${d}" fill="url(#blade)" stroke="${BG_EDGE}" stroke-width="${(R * 0.022).toFixed(
          2
        )}" stroke-linejoin="round"/>`
    )
    .join('\n      ');
  return `
    <g transform="translate(${cx} ${cy})">
      <circle cx="0" cy="0" r="${(R * 1.18).toFixed(2)}" fill="url(#glow)"/>
      ${bladePaths}
      <circle cx="0" cy="0" r="${(r * 0.98).toFixed(2)}" fill="${BG_EDGE}"/>
      <circle cx="0" cy="0" r="${(r * 0.98).toFixed(
        2
      )}" fill="none" stroke="#ffe0a0" stroke-width="${(R * 0.02).toFixed(2)}" opacity="0.85"/>
    </g>`;
}

// ----- icon.svg (full-bleed, foreground on dark bg) -----
const iconSvg = `<svg width="1024" height="1024" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
  <defs>${defs()}</defs>
  <rect width="1024" height="1024" fill="url(#bg)"/>
  ${apertureMark(512, 512, 340)}
</svg>
`;
writeFileSync(join(DIR, 'icon.svg'), iconSvg);

// foreground only (transparent bg, ~18% safe margin => mark radius ~= 1024*0.5*0.82*scale)
// Android adaptive foreground: content should sit within center ~66%. Use R=280.
const fgSvg = `<svg width="1024" height="1024" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
  <defs>${defs()}</defs>
  ${apertureMark(512, 512, 270)}
</svg>
`;

// background only (full-bleed darkroom)
const bgSvg = `<svg width="1024" height="1024" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
  <defs>${defs()}</defs>
  <rect width="1024" height="1024" fill="url(#bg)"/>
  <circle cx="512" cy="512" r="420" fill="url(#glow)" opacity="0.4"/>
</svg>
`;

// splash: centered small mark on darkroom bg
function splashSvg() {
  return `<svg width="2732" height="2732" viewBox="0 0 2732 2732" xmlns="http://www.w3.org/2000/svg">
  <defs>${defs()}</defs>
  <rect width="2732" height="2732" fill="url(#bg)"/>
  ${apertureMark(1366, 1366, 300)}
</svg>
`;
}

async function toPng(svg, out, size) {
  const buf = Buffer.from(svg);
  await sharp(buf, { density: 384 })
    .resize(size, size, { fit: 'cover' })
    .png()
    .toFile(join(DIR, out));
}

async function main() {
  await toPng(iconSvg, 'icon.png', 1024);
  await toPng(fgSvg, 'icon-foreground.png', 1024);
  await toPng(bgSvg, 'icon-background.png', 1024);
  await toPng(splashSvg(), 'splash.png', 2732);
  await toPng(splashSvg(), 'splash-dark.png', 2732);
  console.log('source art written to resources/');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
