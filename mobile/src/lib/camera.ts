// App-level camera service: wraps PtpIpClient into Photo models, thumbnail
// fetching, streamed downloads to the filesystem and device-property control.

import { Directory, Encoding, Filesystem } from '@capacitor/filesystem';
import { PtpIpClient } from './ptpip/client';
import { PropCode } from './ptpip/constants';
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

  async connect(host: string, port = 15740): Promise<void> {
    const client = new PtpIpClient(host, port);
    await client.connect();
    await client.openSession();
    this.client = client;
    this.cameraModel = client.responderName || 'Camera';
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

  async listPhotos(): Promise<Photo[]> {
    const client = this.require();
    const handles = await client.getObjectHandles();
    const photos: Photo[] = [];
    for (const handle of handles) {
      try {
        const info = await client.getObjectInfo(handle);
        // Skip folders / associations.
        if (info.associationType !== 0) continue;
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
        // Ignore individual object failures; keep listing.
      }
    }
    // Newest first.
    photos.sort((a, b) => (b.captureEpochMs ?? 0) - (a.captureEpochMs ?? 0));
    return photos;
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
