import { beforeAll, describe, expect, it, vi } from 'vitest';

// The PtpIpClient imports '@aero-shutter/tcp-socket' which calls
// registerPlugin. We stub that module so the client talks directly to the web
// demo backend, exercising the full handshake -> list -> thumb path.
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

let PtpIpClient: typeof import('../../../src/lib/ptpip/client').PtpIpClient;

beforeAll(async () => {
  ({ PtpIpClient } = await import('../../../src/lib/ptpip/client'));
});

describe('mock camera end-to-end (PTP/IP handshake -> list -> thumb)', () => {
  it('connects, opens a session, lists photos and fetches a thumbnail', async () => {
    const client = new PtpIpClient('127.0.0.1', 15740, 'AeroShutter-Test');
    await client.connect();
    expect(client.responderName).toContain('Nikon');

    await client.openSession();

    const storageIds = await client.getStorageIds();
    expect(storageIds.length).toBeGreaterThan(0);

    const handles = await client.getObjectHandles();
    expect(handles.length).toBe(30);

    const info = await client.getObjectInfo(handles[0]);
    expect(info.filename).toMatch(/DSC_\d+\.(JPG|NEF)/);
    expect(info.objectCompressedSize).toBeGreaterThan(0);

    const thumb = await client.getThumb(handles[0]);
    expect(thumb.length).toBeGreaterThan(0);
    // JPEG SOI marker.
    expect(thumb[0]).toBe(0xff);
    expect(thumb[1]).toBe(0xd8);

    await client.close();
  });

  it('reads and writes a device property (ISO) with re-read confirmation', async () => {
    const client = new PtpIpClient('127.0.0.1', 15740);
    await client.connect();
    await client.openSession();

    const { PropCode } = await import('../../../src/lib/ptpip/constants');
    const desc = await client.getDevicePropDesc(PropCode.ExposureIndex);
    expect(desc.enumValues).toBeDefined();
    const target = desc.enumValues!.find((v) => v !== desc.currentValue)!;

    await client.setDevicePropValue(PropCode.ExposureIndex, desc.dataType, target);
    const after = await client.getDevicePropDesc(PropCode.ExposureIndex);
    expect(after.currentValue).toBe(target);

    await client.close();
  });

  it('streams an object in partial chunks and captures a new frame', async () => {
    const client = new PtpIpClient('127.0.0.1', 15740);
    await client.connect();
    await client.openSession();

    const handles = await client.getObjectHandles();
    const info = await client.getObjectInfo(handles[0]);

    const chunks: number[] = [];
    await client.getObjectStreamed(handles[0], info.objectCompressedSize, async (chunk) => {
      chunks.push(chunk.length);
    });
    const total = chunks.reduce((a, b) => a + b, 0);
    expect(total).toBe(info.objectCompressedSize);

    const before = handles.length;
    await client.initiateCapture();
    const afterHandles = await client.getObjectHandles();
    expect(afterHandles.length).toBe(before + 1);

    await client.close();
  });
});
