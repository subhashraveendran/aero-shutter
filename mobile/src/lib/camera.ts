// App-level camera service: wraps PtpIpClient into Photo models, thumbnail
// fetching, streamed downloads to the filesystem and device-property control.

import { Directory, Encoding, Filesystem } from '@capacitor/filesystem';
import { PtpIpClient, defaultHostName } from './ptpip/client';
import { EventCode, PropCode, TransferListLock, TransferListUnlock } from './ptpip/constants';
import { formatOf, type PhotoFormat } from './ptpip/objectinfo';
import {
  formatPropValue,
  propLabel,
  type DevicePropDesc,
} from './ptpip/devprop';
import { toBase64 } from './base64';
import type { Destination } from './settings';

export interface Photo {
  handle: number;
  filename: string;
  size: number;
  format: PhotoFormat;
  width: number;
  height: number;
  captureEpochMs: number | null;
}

export interface CameraProperty {
  code: number;
  label: string;
  display: string;
  value: number;
  writable: boolean;
  dataType: number;
  options: number[]; // possible values (enum or expanded range)
  desc: DevicePropDesc;
}

const IMPORT_DIR = 'AeroShutter';

/** Property codes shown on the camera-control screen, in display order. */
export const CONTROL_PROPS = [
  PropCode.ExposureIndex, // ISO
  PropCode.FNumber, // aperture
  PropCode.ExposureTime, // shutter
  PropCode.WhiteBalance,
  PropCode.ExposureBiasCompensation, // EV
  PropCode.ExposureProgramMode, // mode
];

function jpegDataUrl(bytes: Uint8Array): string {
  return `data:image/jpeg;base64,${toBase64(bytes)}`;
}

export class CameraService {
  private client: PtpIpClient | null = null;
  cameraModel = '';
  /** How the camera socket was bound to the network ('wifi-bound', etc). */
  networkBinding: string | null = null;

  // App-level callbacks re-applied to each freshly-connected client so they
  // survive reconnects. Set once by the store; the store owns the logic.
  private onObjectAdded: (() => void) | null = null;
  private onDisconnected: ((reason: string) => void) | null = null;

  /**
   * Register a callback fired when the camera reports a new object (fix #3),
   * e.g. a fresh shot. Persisted across reconnects.
   */
  setObjectAddedHandler(cb: (() => void) | null): void {
    this.onObjectAdded = cb;
    this.applyHandlers();
  }

  /**
   * Register a callback fired once when the underlying link is detected dead
   * (fix #4). Persisted across reconnects.
   */
  setDisconnectHandler(cb: ((reason: string) => void) | null): void {
    this.onDisconnected = cb;
    this.applyHandlers();
  }

  /** Wire the current app-level callbacks onto the active client. */
  private applyHandlers(): void {
    const client = this.client;
    if (!client) return;
    client.setEventHandler((event) => {
      if (event.eventCode === EventCode.ObjectAdded) this.onObjectAdded?.();
    });
    client.setDisconnectHandler((reason) => this.onDisconnected?.(reason));
  }

  async connect(host: string, port = 15740, bindWifi = true): Promise<void> {
    const client = new PtpIpClient(host, port, defaultHostName(), bindWifi);
    await client.connect();
    await client.openSession();
    this.client = client;
    this.cameraModel = client.responderName || 'Camera';
    this.networkBinding = client.networkBinding;
    // Re-apply any registered app-level handlers to the new client.
    this.applyHandlers();
  }

  get connected(): boolean {
    return this.client !== null;
  }

  async disconnect(): Promise<void> {
    if (this.client) {
      await this.client.close();
      this.client = null;
    }
  }

  private require(): PtpIpClient {
    if (!this.client) throw new Error('Camera not connected');
    return this.client;
  }

  /**
   * List all photos on the camera. Incremental + resilient:
   * - getObjectHandles() once (fast), then getObjectInfo one handle at a time
   *   (serialized by the client mutex).
   * - Nikon returns handles oldest-first, so we iterate in REVERSE to surface
   *   the NEWEST photos first — the gallery fills top-down with recent shots.
   * - After every small batch we invoke onProgress with the running count and
   *   the freshly-decoded photos so the UI can render progressively instead of
   *   freezing on a spinner while 500+ objects load.
   * - Per-item try/catch: a single bad/hung object is counted and skipped (the
   *   client's 15s transaction timeout guards a truly dead link).
   * Returns the full newest-first list at the end.
   */
  async listPhotos(
    onProgress?: (loaded: number, total: number, newPhotos: Photo[]) => void,
  ): Promise<Photo[]> {
    const client = this.require();
    const handles = await client.getObjectHandles();
    // Reverse: newest (highest handle, latest capture) first.
    const ordered = [...handles].reverse();
    const total = ordered.length;
    const photos: Photo[] = [];
    const BATCH = 8;
    let batch: Photo[] = [];
    let processed = 0;

    for (const handle of ordered) {
      try {
        const info = await client.getObjectInfo(handle);
        // Skip folders / associations.
        if (info.associationType === 0) {
          const photo: Photo = {
            handle,
            filename: info.filename,
            size: info.objectCompressedSize,
            format: formatOf(info.objectFormat, info.filename),
            width: info.imagePixWidth,
            height: info.imagePixHeight,
            captureEpochMs: info.captureEpochMs,
          };
          photos.push(photo);
          batch.push(photo);
        }
      } catch {
        // Ignore individual object failures; keep listing.
      }
      processed += 1;
      if (batch.length >= BATCH || processed === total) {
        onProgress?.(processed, total, batch);
        batch = [];
      }
    }
    // Newest first (handles are already reverse-ordered, but capture time is the
    // authoritative sort key and matches what the store expects).
    photos.sort((a, b) => (b.captureEpochMs ?? 0) - (a.captureEpochMs ?? 0));
    return photos;
  }

  /**
   * Photos the user marked on the camera body for "Send to smart device"
   * (Nikon vendor ops 0x9407 lock / 0x9408 read). Locks the queue, resolves each
   * handle to a Photo, then unlocks. Returns [] on bodies that don't implement
   * the vendor ops (so "unsupported" and "empty queue" look the same). This is
   * the camera-initiated counterpart to ObjectAdded auto-import.
   */
  async transferList(): Promise<Photo[]> {
    const client = this.require();
    try {
      await client.setTransferListLock(TransferListLock);
    } catch {
      return []; // vendor op unsupported: no camera-marked queue
    }
    try {
      const handles = await client.getTransferList();
      const photos: Photo[] = [];
      for (const handle of handles) {
        try {
          const info = await client.getObjectInfo(handle);
          if (info.associationType !== 0) continue; // skip folders
          photos.push({
            handle,
            filename: info.filename,
            size: info.objectCompressedSize,
            format: formatOf(info.objectFormat, info.filename),
            width: info.imagePixWidth,
            height: info.imagePixHeight,
            captureEpochMs: info.captureEpochMs,
          });
        } catch {
          // Skip a single unreadable handle; keep going.
        }
      }
      photos.sort((a, b) => (b.captureEpochMs ?? 0) - (a.captureEpochMs ?? 0));
      return photos;
    } catch {
      return [];
    } finally {
      // Best-effort unlock so we never leave the camera queue locked.
      try {
        await client.setTransferListLock(TransferListUnlock);
      } catch {
        /* ignore */
      }
    }
  }

  /** Lightweight keep-alive round-trip; throws on a dead link. */
  async keepAlive(): Promise<void> {
    await this.require().keepAlive();
  }

  async thumbnail(handle: number): Promise<string> {
    const bytes = await this.require().getThumb(handle);
    return jpegDataUrl(bytes);
  }

  async fullImage(handle: number): Promise<string> {
    const bytes = await this.require().getObject(handle);
    return jpegDataUrl(bytes);
  }

  async capture(): Promise<void> {
    await this.require().initiateCapture();
  }

  /**
   * Download a photo to the chosen destination, streaming 1 MiB chunks to the
   * filesystem. Resumable: if a partial file exists we continue from its size.
   * Reports progress in bytes.
   */
  async importPhoto(
    photo: Photo,
    destination: Destination,
    onProgress: (bytesDone: number, totalBytes: number) => void,
  ): Promise<{ path: string }> {
    const client = this.require();
    const dir = destination === 'gallery' ? Directory.Data : Directory.Documents;
    const path = `${IMPORT_DIR}/${photo.filename}`;

    let startOffset = 0;
    try {
      const stat = await Filesystem.stat({ path, directory: dir });
      if (typeof stat.size === 'number' && stat.size < photo.size) {
        startOffset = stat.size; // resume
      } else if (stat.size === photo.size) {
        onProgress(photo.size, photo.size);
        return { path };
      }
    } catch {
      // No partial file: start fresh.
      await Filesystem.writeFile({
        path,
        directory: dir,
        data: '',
        encoding: Encoding.UTF8,
        recursive: true,
      });
    }

    onProgress(startOffset, photo.size);
    await client.getObjectStreamed(
      photo.handle,
      photo.size,
      async (chunk, offset) => {
        await Filesystem.appendFile({
          path,
          directory: dir,
          data: toBase64(chunk),
        });
        onProgress(offset + chunk.length, photo.size);
      },
      startOffset,
    );

    return { path };
  }

  async readProperty(code: number): Promise<CameraProperty | null> {
    try {
      const desc = await this.require().getDevicePropDesc(code);
      const options = desc.enumValues ?? expandRange(desc);
      return {
        code,
        label: propLabel(code),
        display: formatPropValue(code, desc.currentValue),
        value: desc.currentValue,
        writable: desc.getSet === 1,
        dataType: desc.dataType,
        options,
        desc,
      };
    } catch {
      return null;
    }
  }

  async readControlProps(): Promise<CameraProperty[]> {
    const out: CameraProperty[] = [];
    for (const code of CONTROL_PROPS) {
      const prop = await this.readProperty(code);
      if (prop) out.push(prop);
    }
    return out;
  }

  /** Write a property value, then re-read to confirm. Returns the fresh prop. */
  async writeProperty(prop: CameraProperty, value: number): Promise<CameraProperty | null> {
    await this.require().setDevicePropValue(prop.code, prop.dataType, value);
    return this.readProperty(prop.code);
  }
}

function expandRange(desc: DevicePropDesc): number[] {
  if (desc.formFlag !== 0x01 || !desc.range) return [];
  const { min, max, step } = desc.range;
  if (step <= 0) return [min, max];
  const out: number[] = [];
  for (let v = min; v <= max && out.length < 128; v += step) out.push(v);
  return out;
}

export const camera = new CameraService();
