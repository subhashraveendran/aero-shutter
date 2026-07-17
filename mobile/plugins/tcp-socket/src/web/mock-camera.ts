// A browser-side mock camera that speaks enough PTP/IP to run the whole app in
// demo mode. It consumes framed request packets and produces framed response
// packets, driving the same PtpIpClient used against real hardware.

import {
  DataPhase,
  DataType,
  OpCode,
  PacketType,
  PropCode,
  RespCode,
} from '../../../../src/lib/ptpip/constants';
import { ByteReader, ByteWriter } from '../../../../src/lib/ptpip/buffer';
import {
  decodeOperationRequest,
  decodePackets,
  encodeEndData,
  encodeInitCommandRequest,
  encodeOperationResponse,
  encodeStartData,
  type Packet,
} from '../../../../src/lib/ptpip/packets';
import { encodeObjectInfo, type ObjectInfo } from '../../../../src/lib/ptpip/objectinfo';
import { encodeDevicePropDesc, type DevicePropDesc } from '../../../../src/lib/ptpip/devprop';
import { formatPtpDateTime } from '../../../../src/lib/ptpip/buffer';
import { generateDemoPhotos, type DemoPhoto } from './demo-photos';

const STORAGE_ID = 0x00010001;

interface MutableProp {
  desc: DevicePropDesc;
}

function u32ArrayPayload(values: number[]): Uint8Array {
  const w = new ByteWriter();
  w.u32(values.length);
  for (const v of values) w.u32(v);
  return w.toUint8Array();
}

export class MockCameraSocket {
  private buffer = new Uint8Array(0);
  private photos: DemoPhoto[];
  private props = new Map<number, MutableProp>();
  private pendingDataOp: { opcode: number; txn: number; params: number[] } | null = null;

  constructor(private emit: (event: 'data' | 'closed' | 'error', payload: unknown) => void) {
    this.photos = generateDemoPhotos();
    this.initProps();
  }

  private initProps(): void {
    const enumProp = (
      propCode: number,
      dataType: number,
      current: number,
      values: number[],
      getSet = 1,
    ): void => {
      this.props.set(propCode, {
        desc: {
          propCode,
          dataType,
          getSet,
          factoryDefault: values[0],
          currentValue: current,
          formFlag: 0x02,
          enumValues: values,
        },
      });
    };

    // ISO (enum).
    enumProp(PropCode.ExposureIndex, DataType.UINT16, 400, [100, 200, 400, 800, 1600, 3200, 6400]);
    // Aperture (FNumber, u16/100).
    enumProp(PropCode.FNumber, DataType.UINT16, 560, [280, 350, 400, 560, 800, 1100, 1600]);
    // Shutter (ExposureTime, 0.1ms units).
    enumProp(
      PropCode.ExposureTime,
      DataType.UINT32,
      40, // 1/250s
      [10000, 1250, 400, 100, 40, 20, 10, 5],
    );
    // White balance (enum incl. Nikon vendor codes).
    enumProp(PropCode.WhiteBalance, DataType.UINT16, 2, [
      2, 4, 5, 6, 7, 32784, 32785, 32786, 32787,
    ]);
    // Exposure compensation (int16 millistops, range form).
    this.props.set(PropCode.ExposureBiasCompensation, {
      desc: {
        propCode: PropCode.ExposureBiasCompensation,
        dataType: DataType.INT16,
        getSet: 1,
        factoryDefault: 0,
        currentValue: 0,
        formFlag: 0x01,
        range: { min: -3000, max: 3000, step: 500 },
      },
    });
    // Exposure program mode (read-only enum).
    enumProp(PropCode.ExposureProgramMode, DataType.UINT16, 3, [1, 2, 3, 4], 0);
    // Battery level (read-only).
    this.props.set(PropCode.BatteryLevel, {
      desc: {
        propCode: PropCode.BatteryLevel,
        dataType: DataType.UINT8,
        getSet: 0,
        factoryDefault: 100,
        currentValue: 87,
        formFlag: 0x01,
        range: { min: 0, max: 100, step: 1 },
      },
    });
  }

  /** Feed base64-decoded bytes written by the client. */
  receive(data: Uint8Array): void {
    const merged = new Uint8Array(this.buffer.length + data.length);
    merged.set(this.buffer, 0);
    merged.set(data, this.buffer.length);
    this.buffer = merged;
    const { packets, consumed } = decodePackets(this.buffer);
    this.buffer = this.buffer.subarray(consumed);
    for (const p of packets) this.handle(p);
  }

  private send(bytes: Uint8Array): void {
    // Emit asynchronously to mimic real socket delivery.
    queueMicrotask(() => this.emit('data', bytes));
  }

  private handle(p: Packet): void {
    switch (p.type) {
      case PacketType.InitCommandRequest:
        this.handleInitCommand();
        break;
      case PacketType.InitEventRequest:
        this.send(encodeInitEventAck());
        break;
      case PacketType.OperationRequest:
        this.handleOperation(p.payload);
        break;
      case PacketType.StartData:
        break;
      case PacketType.Data:
      case PacketType.EndData:
        this.handleDataPhase(p.payload);
        break;
      case PacketType.ProbeRequest:
        this.send(encodeProbeResponse());
        break;
      default:
        break;
    }
  }

  private handleInitCommand(): void {
    // Reply with an InitCommandAck: connectionNumber + 16-byte GUID + name.
    const w = new ByteWriter();
    w.u32(1); // connection number
    const guid = new Uint8Array(16);
    for (let i = 0; i < 16; i++) guid[i] = i + 1;
    w.bytes(guid);
    for (const ch of [...'Nikon D5300 (Demo)']) w.u16(ch.codePointAt(0) ?? 0);
    w.u16(0);
    this.send(frame(PacketType.InitCommandAck, w.toUint8Array()));
  }

  private handleOperation(payload: Uint8Array): void {
    const req = decodeOperationRequest(payload);
    switch (req.opcode) {
      case OpCode.OpenSession:
        return this.respond(req.transactionId, RespCode.OK);
      case OpCode.CloseSession:
        return this.respond(req.transactionId, RespCode.OK);
      case OpCode.GetStorageIDs:
        return this.respondData(req.transactionId, u32ArrayPayload([STORAGE_ID]));
      case OpCode.GetObjectHandles:
        return this.respondData(
          req.transactionId,
          u32ArrayPayload(this.photos.map((ph) => ph.handle)),
        );
      case OpCode.GetObjectInfo:
        return this.handleGetObjectInfo(req.transactionId, req.params[0]);
      case OpCode.GetThumb:
      case OpCode.NikonGetLargeThumb:
        return this.handleGetThumb(req.transactionId, req.params[0], req.opcode);
      case OpCode.GetObject:
        return this.handleGetObject(req.transactionId, req.params[0]);
      case OpCode.GetPartialObject:
        return this.handleGetPartialObject(
          req.transactionId,
          req.params[0],
          req.params[1],
          req.params[2],
        );
      case OpCode.GetDevicePropDesc:
        return this.handleGetDevicePropDesc(req.transactionId, req.params[0]);
      case OpCode.GetDevicePropValue:
        return this.handleGetDevicePropValue(req.transactionId, req.params[0]);
      case OpCode.SetDevicePropValue:
        // Data phase follows; stash the pending op.
        this.pendingDataOp = { opcode: req.opcode, txn: req.transactionId, params: req.params };
        return;
      case OpCode.InitiateCapture:
        return this.handleInitiateCapture(req.transactionId);
      default:
        return this.respond(req.transactionId, RespCode.OperationNotSupported);
    }
  }

  private handleDataPhase(payload: Uint8Array): void {
    if (!this.pendingDataOp) return;
    const r = new ByteReader(payload);
    r.u32(); // transaction id
    const valueBytes = payload.subarray(4);
    const op = this.pendingDataOp;
    this.pendingDataOp = null;

    if (op.opcode === OpCode.SetDevicePropValue) {
      const propCode = op.params[0];
      const prop = this.props.get(propCode);
      if (!prop || prop.desc.getSet !== 1) {
        return this.respond(op.txn, RespCode.AccessDenied);
      }
      const value = decodeByType(valueBytes, prop.desc.dataType);
      prop.desc.currentValue = value;
      return this.respond(op.txn, RespCode.OK);
    }
    this.respond(op.txn, RespCode.OK);
  }

  private photoByHandle(handle: number): DemoPhoto | undefined {
    return this.photos.find((p) => p.handle === handle);
  }

  private handleGetObjectInfo(txn: number, handle: number): void {
    const ph = this.photoByHandle(handle);
    if (!ph) return this.respond(txn, RespCode.InvalidObjectHandle);
    const info: ObjectInfo = {
      storageId: STORAGE_ID,
      objectFormat: ph.objectFormat,
      protectionStatus: 0,
      objectCompressedSize: ph.full.length,
      thumbFormat: 0x3801,
      thumbCompressedSize: ph.thumb.length,
      thumbPixWidth: 160,
      thumbPixHeight: 107,
      imagePixWidth: ph.width,
      imagePixHeight: ph.height,
      imageBitDepth: 24,
      parentObject: 0,
      associationType: 0,
      associationDesc: 0,
      sequenceNumber: 0,
      filename: ph.filename,
      captureDate: formatPtpDateTime(ph.captureEpochMs),
      modificationDate: formatPtpDateTime(ph.captureEpochMs),
      keywords: '',
      captureEpochMs: ph.captureEpochMs,
    };
    this.respondData(txn, encodeObjectInfo(info));
  }

  private handleGetThumb(txn: number, handle: number, opcode: number): void {
    const ph = this.photoByHandle(handle);
    if (!ph) return this.respond(txn, RespCode.InvalidObjectHandle);
    // For the Nikon large-thumb opcode, hand back a bigger image (the full JPEG)
    // to exercise the attempt-with-fallback path.
    const data = opcode === OpCode.NikonGetLargeThumb ? ph.full : ph.thumb;
    this.respondData(txn, data);
  }

  private handleGetObject(txn: number, handle: number): void {
    const ph = this.photoByHandle(handle);
    if (!ph) return this.respond(txn, RespCode.InvalidObjectHandle);
    this.respondData(txn, ph.full);
  }

  private handleGetPartialObject(txn: number, handle: number, offset: number, count: number): void {
    const ph = this.photoByHandle(handle);
    if (!ph) return this.respond(txn, RespCode.InvalidObjectHandle);
    const end = Math.min(ph.full.length, offset + count);
    const slice = ph.full.subarray(Math.min(offset, ph.full.length), end);
    this.respondData(txn, new Uint8Array(slice), [slice.length]);
  }

  private handleGetDevicePropDesc(txn: number, propCode: number): void {
    const prop = this.props.get(propCode);
    if (!prop) return this.respond(txn, RespCode.DevicePropNotSupported);
    this.respondData(txn, encodeDevicePropDesc(prop.desc));
  }

  private handleGetDevicePropValue(txn: number, propCode: number): void {
    const prop = this.props.get(propCode);
    if (!prop) return this.respond(txn, RespCode.DevicePropNotSupported);
    const w = new ByteWriter();
    writeByType(w, prop.desc.dataType, prop.desc.currentValue);
    this.respondData(txn, w.toUint8Array());
  }

  private handleInitiateCapture(txn: number): void {
    // Add a fresh demo shot so a gallery refresh shows a new photo.
    const idx = this.photos.length;
    const base = this.photos[idx % Math.max(1, this.photos.length - 1)] ?? this.photos[0];
    if (base) {
      const clone: DemoPhoto = {
        ...base,
        handle: 0x20000 + idx,
        filename: `DSC_${2000 + idx}.JPG`,
        format: 'JPEG',
        objectFormat: 0x3801,
        captureEpochMs: Date.now(),
      };
      this.photos.push(clone);
    }
    this.respond(txn, RespCode.OK);
  }

  private respond(txn: number, code: number, params: number[] = []): void {
    this.send(encodeOperationResponse({ responseCode: code, transactionId: txn, params }));
  }

  private respondData(txn: number, data: Uint8Array, params: number[] = []): void {
    this.send(encodeStartData(txn, data.length));
    this.send(encodeEndData(txn, data));
    this.respond(txn, RespCode.OK, params);
  }

  destroy(): void {
    this.emit('closed', {});
  }
}

// ---- framing helpers (kept local to avoid importing the client) -----------

function frame(type: number, payload: Uint8Array): Uint8Array {
  const w = new ByteWriter();
  w.u32(payload.length + 8);
  w.u32(type);
  w.bytes(payload);
  return w.toUint8Array();
}

function encodeInitEventAck(): Uint8Array {
  return frame(PacketType.InitEventAck, new Uint8Array(0));
}

function encodeProbeResponse(): Uint8Array {
  return frame(PacketType.ProbeResponse, new Uint8Array(0));
}

function decodeByType(bytes: Uint8Array, dataType: number): number {
  const r = new ByteReader(bytes);
  return readByType(r, dataType);
}

function readByType(r: ByteReader, dataType: number): number {
  switch (dataType) {
    case DataType.INT8:
      return r.i8();
    case DataType.UINT8:
      return r.u8();
    case DataType.INT16:
      return r.i16();
    case DataType.UINT16:
      return r.u16();
    case DataType.INT32:
      return r.i32();
    case DataType.UINT32:
      return r.u32();
    default:
      return r.u16();
  }
}

function writeByType(w: ByteWriter, dataType: number, value: number): void {
  switch (dataType) {
    case DataType.INT8:
      w.i8(value);
      break;
    case DataType.UINT8:
      w.u8(value);
      break;
    case DataType.INT16:
      w.i16(value);
      break;
    case DataType.UINT16:
      w.u16(value);
      break;
    case DataType.INT32:
      w.i32(value);
      break;
    case DataType.UINT32:
      w.u32(value);
      break;
    default:
      w.u16(value);
  }
}

// Referenced to keep tree-shaking honest; the client encodes these itself.
export const __demoFramingRefs = {
  encodeInitCommandRequest,
  DataPhase,
  encodeInitEventAck,
};
