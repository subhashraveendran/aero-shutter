// ObjectInfo dataset parsing/encoding (PTP standard layout).

import { ByteReader, ByteWriter, parsePtpDateTime } from './buffer';
import { ObjectFormat } from './constants';

export interface ObjectInfo {
  storageId: number;
  objectFormat: number;
  protectionStatus: number;
  objectCompressedSize: number;
  thumbFormat: number;
  thumbCompressedSize: number;
  thumbPixWidth: number;
  thumbPixHeight: number;
  imagePixWidth: number;
  imagePixHeight: number;
  imageBitDepth: number;
  parentObject: number;
  associationType: number;
  associationDesc: number;
  sequenceNumber: number;
  filename: string;
  captureDate: string;
  modificationDate: string;
  keywords: string;
  /** Convenience: parsed capture time in epoch millis, or null. */
  captureEpochMs: number | null;
}

export function parseObjectInfo(payload: Uint8Array): ObjectInfo {
  const r = new ByteReader(payload);
  const storageId = r.u32();
  const objectFormat = r.u16();
  const protectionStatus = r.u16();
  const objectCompressedSize = r.u32();
  const thumbFormat = r.u16();
  const thumbCompressedSize = r.u32();
  const thumbPixWidth = r.u32();
  const thumbPixHeight = r.u32();
  const imagePixWidth = r.u32();
  const imagePixHeight = r.u32();
  const imageBitDepth = r.u32();
  const parentObject = r.u32();
  const associationType = r.u16();
  const associationDesc = r.u32();
  const sequenceNumber = r.u32();
  const filename = r.ptpString();
  const captureDate = r.ptpString();
  const modificationDate = r.ptpString();
  const keywords = r.ptpString();

  return {
    storageId,
    objectFormat,
    protectionStatus,
    objectCompressedSize,
    thumbFormat,
    thumbCompressedSize,
    thumbPixWidth,
    thumbPixHeight,
    imagePixWidth,
    imagePixHeight,
    imageBitDepth,
    parentObject,
    associationType,
    associationDesc,
    sequenceNumber,
    filename,
    captureDate,
    modificationDate,
    keywords,
    captureEpochMs: parsePtpDateTime(captureDate),
  };
}

export function encodeObjectInfo(info: ObjectInfo): Uint8Array {
  const w = new ByteWriter();
  w.u32(info.storageId);
  w.u16(info.objectFormat);
  w.u16(info.protectionStatus);
  w.u32(info.objectCompressedSize);
  w.u16(info.thumbFormat);
  w.u32(info.thumbCompressedSize);
  w.u32(info.thumbPixWidth);
  w.u32(info.thumbPixHeight);
  w.u32(info.imagePixWidth);
  w.u32(info.imagePixHeight);
  w.u32(info.imageBitDepth);
  w.u32(info.parentObject);
  w.u16(info.associationType);
  w.u32(info.associationDesc);
  w.u32(info.sequenceNumber);
  w.ptpString(info.filename);
  w.ptpString(info.captureDate);
  w.ptpString(info.modificationDate);
  w.ptpString(info.keywords);
  return w.toUint8Array();
}

export type PhotoFormat = 'RAW' | 'JPEG' | 'OTHER';

export function formatOf(objectFormat: number, filename: string): PhotoFormat {
  if (objectFormat === ObjectFormat.EXIF_JPEG) return 'JPEG';
  if (objectFormat === ObjectFormat.TIFF_EP || objectFormat === ObjectFormat.NEF) return 'RAW';
  const lower = filename.toLowerCase();
  if (lower.endsWith('.nef') || lower.endsWith('.raw') || lower.endsWith('.tif')) return 'RAW';
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'JPEG';
  return 'OTHER';
}
