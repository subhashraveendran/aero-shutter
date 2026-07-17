// Generates ~30 demo photos (canvas-rendered JPEG scenes) with plausible
// Nikon-style metadata, used to power browser demo mode end-to-end.

export interface DemoPhoto {
  handle: number;
  filename: string;
  format: 'JPEG' | 'RAW';
  objectFormat: number;
  width: number;
  height: number;
  captureEpochMs: number;
  full: Uint8Array; // full-resolution JPEG (RAW entries also carry a JPEG preview)
  thumb: Uint8Array; // small JPEG thumbnail
}

const SCENES: Array<{ name: string; from: string; to: string; accent: string }> = [
  { name: 'Dawn Ridge', from: '#1a2a44', to: '#e8794a', accent: '#ffd27a' },
  { name: 'Neon Alley', from: '#12021f', to: '#5b1f8a', accent: '#39e6ff' },
  { name: 'Salt Flats', from: '#dfe7ef', to: '#8fa6c4', accent: '#ffffff' },
  { name: 'Pine Fog', from: '#0e1a14', to: '#4a6b58', accent: '#bfe3c8' },
  { name: 'Harbor Dusk', from: '#0b1e33', to: '#c65b4e', accent: '#ffb15c' },
  { name: 'Desert Noon', from: '#c98b4a', to: '#f0d9a8', accent: '#7a4a1f' },
  { name: 'Aurora', from: '#03121f', to: '#1f8a6b', accent: '#7affd6' },
  { name: 'City Rain', from: '#141821', to: '#38506b', accent: '#8ab4ff' },
  { name: 'Canyon Gold', from: '#3a1a10', to: '#c9762f', accent: '#ffce8a' },
  { name: 'Snow Peak', from: '#8fb0d4', to: '#ffffff', accent: '#c0d8f0' },
];

function drawScene(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  scene: (typeof SCENES)[number],
  seed: number,
): void {
  const grad = ctx.createLinearGradient(0, 0, w, h);
  grad.addColorStop(0, scene.from);
  grad.addColorStop(1, scene.to);
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, w, h);

  // Pseudo-random deterministic hills / shapes.
  let s = seed * 9301 + 49297;
  const rnd = () => {
    s = (s * 9301 + 49297) % 233280;
    return s / 233280;
  };

  // Sun / moon.
  ctx.globalAlpha = 0.9;
  ctx.fillStyle = scene.accent;
  const cx = w * (0.2 + rnd() * 0.6);
  const cy = h * (0.15 + rnd() * 0.25);
  ctx.beginPath();
  ctx.arc(cx, cy, Math.min(w, h) * 0.07, 0, Math.PI * 2);
  ctx.fill();
  ctx.globalAlpha = 1;

  // Layered ridgelines.
  const layers = 3;
  for (let l = 0; l < layers; l++) {
    const baseY = h * (0.55 + l * 0.13);
    ctx.beginPath();
    ctx.moveTo(0, baseY);
    for (let x = 0; x <= w; x += w / 8) {
      const y = baseY - rnd() * h * 0.12 * (layers - l);
      ctx.lineTo(x, y);
    }
    ctx.lineTo(w, h);
    ctx.lineTo(0, h);
    ctx.closePath();
    const shade = Math.floor(20 + l * 22);
    ctx.fillStyle = `rgba(${shade}, ${shade + 8}, ${shade + 18}, ${0.55 + l * 0.15})`;
    ctx.fill();
  }
}

function canvasToBytes(canvas: HTMLCanvasElement, quality: number): Uint8Array {
  const dataUrl = canvas.toDataURL('image/jpeg', quality);
  const base64 = dataUrl.split(',')[1] ?? '';
  const binary = atob(base64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

function canvasAvailable(): boolean {
  if (typeof document === 'undefined') return false;
  // jsdom (test env) exposes canvas but throws "not implemented" from
  // getContext. Silence its console noise while we probe.
  const origError = console.error;
  console.error = () => {};
  try {
    const c = document.createElement('canvas');
    return (
      typeof c.getContext === 'function' &&
      !!c.getContext('2d') &&
      typeof c.toDataURL === 'function'
    );
  } catch {
    return false;
  } finally {
    console.error = origError;
  }
}

// A minimal but valid 1x1 JPEG, used when a real canvas isn't available
// (e.g. jsdom test environment). Keeps the mock end-to-end functional.
const PLACEHOLDER_JPEG = Uint8Array.from(
  atob(
    '/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAMCAgICAgMCAgIDAwMDBAYEBAQEBAgGBgUGCQgKCgkICQkKDA8MCgsOCwkJDRENDg8QEBEQCgwSExIQEw8QEBD/2wBDAQMDAwQDBAgEBAgQCwkLEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBD/wAARCAABAAEDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwD8/wCiiigD/9k=',
  ),
  (c) => c.charCodeAt(0),
);

let cache: DemoPhoto[] | null = null;

export function generateDemoPhotos(): DemoPhoto[] {
  if (cache) return cache;
  const photos: DemoPhoto[] = [];
  const count = 30;
  const now = Date.now();
  const haveCanvas = canvasAvailable();

  for (let i = 0; i < count; i++) {
    const scene = SCENES[i % SCENES.length];
    const isRaw = i % 3 === 0; // every third shot is a NEF
    const fullW = 480;
    const fullH = 320;

    let fullBytes: Uint8Array;
    let thumbBytes: Uint8Array;

    if (haveCanvas) {
      const full = document.createElement('canvas');
      full.width = fullW;
      full.height = fullH;
      const fctx = full.getContext('2d')!;
      drawScene(fctx, fullW, fullH, scene, i + 1);
      fctx.fillStyle = 'rgba(255,255,255,0.85)';
      fctx.font = '16px sans-serif';
      fctx.fillText(`${scene.name} #${String(i + 1).padStart(2, '0')}`, 14, fullH - 16);

      const thumbC = document.createElement('canvas');
      thumbC.width = 160;
      thumbC.height = 107;
      const tctx = thumbC.getContext('2d')!;
      tctx.drawImage(full, 0, 0, 160, 107);

      fullBytes = canvasToBytes(full, 0.82);
      thumbBytes = canvasToBytes(thumbC, 0.7);
    } else {
      fullBytes = PLACEHOLDER_JPEG;
      thumbBytes = PLACEHOLDER_JPEG;
    }

    const num = 1000 + i;
    photos.push({
      handle: 0x10000 + i,
      filename: `DSC_${num}.${isRaw ? 'NEF' : 'JPG'}`,
      format: isRaw ? 'RAW' : 'JPEG',
      objectFormat: isRaw ? 0x3802 : 0x3801,
      width: fullW,
      height: fullH,
      captureEpochMs: now - (count - i) * 137_000,
      full: fullBytes,
      thumb: thumbBytes,
    });
  }

  cache = photos;
  return photos;
}
