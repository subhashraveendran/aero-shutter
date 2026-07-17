import { describe, expect, it } from 'vitest';
import { ByteReader, ByteWriter, parsePtpDateTime } from './buffer';
import { encodeObjectInfo, formatOf, parseObjectInfo, type ObjectInfo } from './objectinfo';

describe('PTP string codec', () => {
  it('round-trips a unicode string through ByteWriter/ByteReader', () => {
    const w = new ByteWriter();
    w.ptpString('DSC_1001.NEF');
    const r = new ByteReader(w.toUint8Array());
    expect(r.ptpString()).toBe('DSC_1001.NEF');
  });

  it('encodes empty string as a single zero length byte', () => {
    const w = new ByteWriter();
    w.ptpString('');
    expect([...w.toUint8Array()]).toEqual([0]);
  });
});

describe('ObjectInfo parsing', () => {
  const info: ObjectInfo = {
    storageId: 0x00010001,
    objectFormat: 0x3801,
    protectionStatus: 0,
    objectCompressedSize: 8_500_000,
    thumbFormat: 0x3801,
    thumbCompressedSize: 12000,
    thumbPixWidth: 160,
    thumbPixHeight: 120,
    imagePixWidth: 6000,
    imagePixHeight: 4000,
    imageBitDepth: 24,
    parentObject: 0,
    associationType: 0,
    associationDesc: 0,
    sequenceNumber: 0,
    filename: 'DSC_2043.JPG',
    captureDate: '20260717T083015',
    modificationDate: '20260717T083015',
    keywords: '',
    captureEpochMs: parsePtpDateTime('20260717T083015'),
  };

  it('round-trips an ObjectInfo dataset', () => {
    const encoded = encodeObjectInfo(info);
    const decoded = parseObjectInfo(encoded);
    expect(decoded.filename).toBe('DSC_2043.JPG');
    expect(decoded.objectCompressedSize).toBe(8_500_000);
    expect(decoded.imagePixWidth).toBe(6000);
    expect(decoded.captureDate).toBe('20260717T083015');
    expect(decoded.captureEpochMs).not.toBeNull();
  });

  it('parses PTP datetimes', () => {
    const ms = parsePtpDateTime('20260717T083015');
    expect(ms).not.toBeNull();
    const d = new Date(ms!);
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(6);
    expect(d.getDate()).toBe(17);
  });

  it('classifies formats', () => {
    expect(formatOf(0x3801, 'DSC.JPG')).toBe('JPEG');
    expect(formatOf(0x3802, 'DSC.NEF')).toBe('RAW');
    expect(formatOf(0x9999, 'DSC.NEF')).toBe('RAW');
    expect(formatOf(0x9999, 'DSC.mov')).toBe('OTHER');
  });
});
