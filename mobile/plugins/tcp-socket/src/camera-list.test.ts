import { beforeAll, describe, expect, it, vi } from 'vitest';

// Exercise CameraService.listPhotos() progressive + newest-first behavior
// against the web demo backend (same wiring as mock-e2e).
import { TcpSocketWeb } from './web';

const web = new TcpSocketWeb();

vi.mock('@aero-shutter/tcp-socket', () => {
  return {
    TcpSocket: {
      connect: (o: { host: string; port: number; timeoutMs?: number }) => web.connect(o),
      write: (o: { socketId: string; dataB64: string }) => web.write(o),
      close: (o: { socketId: string }) => web.close(o),
      addListener: (event: string, cb: (e: unknown) => void) => web.addListener(event, cb),
    },
    isDemoMode: () => true,
  };
});

// Filesystem is only touched by import paths, not listPhotos; stub to be safe.
vi.mock('@capacitor/filesystem', () => ({
  Directory: { Data: 'DATA', Documents: 'DOCUMENTS' },
  Encoding: { UTF8: 'utf8' },
  Filesystem: {},
}));

let CameraService: typeof import('../../../src/lib/camera').CameraService;
type Photo = import('../../../src/lib/camera').Photo;

beforeAll(async () => {
  ({ CameraService } = await import('../../../src/lib/camera'));
});

describe('CameraService.listPhotos progressive listing', () => {
  it('reports progress in batches and returns newest-first', async () => {
    const svc = new CameraService();
    await svc.connect('127.0.0.1', 15740, true);

    const progressCalls: Array<{ loaded: number; total: number; batch: number }> = [];
    let seenNew = 0;
    const photos = await svc.listPhotos((loaded, total, newPhotos) => {
      progressCalls.push({ loaded, total, batch: newPhotos.length });
      seenNew += newPhotos.length;
    });

    // 30 demo photos.
    expect(photos.length).toBe(30);
    // Progress was reported incrementally (more than one call for 30 items).
    expect(progressCalls.length).toBeGreaterThan(1);
    // Final call reports loaded === total.
    const last = progressCalls[progressCalls.length - 1];
    expect(last.loaded).toBe(last.total);
    expect(last.total).toBe(30);
    // Every decoded photo was surfaced through onProgress exactly once.
    expect(seenNew).toBe(photos.length);
    // Sorted newest-first by capture time.
    for (let i = 1; i < photos.length; i++) {
      const prev = photos[i - 1] as Photo;
      const cur = photos[i] as Photo;
      expect(prev.captureEpochMs ?? 0).toBeGreaterThanOrEqual(cur.captureEpochMs ?? 0);
    }

    await svc.disconnect();
  });

  it('works without a progress callback', async () => {
    const svc = new CameraService();
    await svc.connect('127.0.0.1', 15740, true);
    const photos = await svc.listPhotos();
    expect(photos.length).toBe(30);
    await svc.disconnect();
  });
});
