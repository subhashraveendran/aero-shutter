// PTP/IP packet framing: little-endian {u32 length, u32 type, payload}.
// length includes the 8-byte header.

import { ByteReader, ByteWriter } from './buffer';
import { DataPhase, PTPIP_VERSION, PacketType } from './constants';

export interface Packet {
  type: number;
  payload: Uint8Array;
}

/** Frame a packet: length (incl. 8-byte header) + type + payload. */
export function encodePacket(type: number, payload: Uint8Array): Uint8Array {
  const w = new ByteWriter();
  w.u32(payload.length + 8);
  w.u32(type);
  w.bytes(payload);
  return w.toUint8Array();
}

/**
 * Decode as many complete packets as available from a buffer.
 * Returns the decoded packets and the number of bytes consumed. Partial
 * trailing data is left for the caller to re-buffer.
 */
export function decodePackets(buffer: Uint8Array): { packets: Packet[]; consumed: number } {
  const packets: Packet[] = [];
  let offset = 0;
  while (buffer.length - offset >= 8) {
    const r = new ByteReader(buffer.subarray(offset));
    const len = r.u32();
    if (len < 8) {
      // Corrupt frame; skip the header to avoid an infinite loop.
      offset += 8;
      continue;
    }
    if (buffer.length - offset < len) break; // incomplete
    const type = r.u32();
    const payload = buffer.subarray(offset + 8, offset + len);
    packets.push({ type, payload: new Uint8Array(payload) });
    offset += len;
  }
  return { packets, consumed: offset };
}

// A 16-byte host GUID + a name; used on both init requests.
export function encodeInitCommandRequest(guid: Uint8Array, name: string): Uint8Array {
  const w = new ByteWriter();
  w.bytes(guid.subarray(0, 16));
  // Host friendly name as UTF-16LE + NUL terminator.
  for (const ch of [...name]) w.u16(ch.codePointAt(0) ?? 0);
  w.u16(0);
  w.u32(PTPIP_VERSION);
  return encodePacket(PacketType.InitCommandRequest, w.toUint8Array());
}

export interface InitCommandAck {
  connectionNumber: number;
  guid: Uint8Array;
  name: string;
}

export function decodeInitCommandAck(payload: Uint8Array): InitCommandAck {
  const r = new ByteReader(payload);
  const connectionNumber = r.u32();
  const guid = r.bytes(16);
  // Responder friendly name, UTF-16LE NUL-terminated.
  let name = '';
  while (r.remaining >= 2) {
    const code = r.u16();
    if (code === 0) break;
    name += String.fromCodePoint(code);
  }
  return { connectionNumber, guid: new Uint8Array(guid), name };
}

export function encodeInitEventRequest(connectionNumber: number): Uint8Array {
  const w = new ByteWriter();
  w.u32(connectionNumber);
  return encodePacket(PacketType.InitEventRequest, w.toUint8Array());
}

export interface OperationRequest {
  dataPhase: number;
  opcode: number;
  transactionId: number;
  params: number[];
}

export function encodeOperationRequest(req: OperationRequest): Uint8Array {
  const w = new ByteWriter();
  w.u32(req.dataPhase);
  w.u16(req.opcode);
  w.u32(req.transactionId);
  for (const p of req.params) w.u32(p);
  return encodePacket(PacketType.OperationRequest, w.toUint8Array());
}

export function decodeOperationRequest(payload: Uint8Array): OperationRequest {
  const r = new ByteReader(payload);
  const dataPhase = r.u32();
  const opcode = r.u16();
  const transactionId = r.u32();
  const params: number[] = [];
  while (r.remaining >= 4) params.push(r.u32());
  return { dataPhase, opcode, transactionId, params };
}

export interface OperationResponse {
  responseCode: number;
  transactionId: number;
  params: number[];
}

export function encodeOperationResponse(resp: OperationResponse): Uint8Array {
  const w = new ByteWriter();
  w.u16(resp.responseCode);
  w.u32(resp.transactionId);
  for (const p of resp.params) w.u32(p);
  return encodePacket(PacketType.OperationResponse, w.toUint8Array());
}

export function decodeOperationResponse(payload: Uint8Array): OperationResponse {
  const r = new ByteReader(payload);
  const responseCode = r.u16();
  const transactionId = r.u32();
  const params: number[] = [];
  while (r.remaining >= 4) params.push(r.u32());
  return { responseCode, transactionId, params };
}

export interface EventPacket {
  eventCode: number;
  transactionId: number;
  params: number[];
}

// Event packet payload: eventCode (u16) + transactionId (u32) + up to 3 u32
// params. Some responders omit the transactionId/params; we read defensively.
export function decodeEvent(payload: Uint8Array): EventPacket {
  const r = new ByteReader(payload);
  const eventCode = r.remaining >= 2 ? r.u16() : 0;
  const transactionId = r.remaining >= 4 ? r.u32() : 0;
  const params: number[] = [];
  while (r.remaining >= 4) params.push(r.u32());
  return { eventCode, transactionId, params };
}

// StartData: transactionId (u32) + total length (u64).
export function encodeStartData(transactionId: number, totalLength: number): Uint8Array {
  const w = new ByteWriter();
  w.u32(transactionId);
  w.u32(totalLength >>> 0);
  w.u32(Math.floor(totalLength / 0x100000000));
  return encodePacket(PacketType.StartData, w.toUint8Array());
}

export function decodeStartData(payload: Uint8Array): { transactionId: number; totalLength: number } {
  const r = new ByteReader(payload);
  const transactionId = r.u32();
  const lo = r.u32();
  const hi = r.u32();
  return { transactionId, totalLength: hi * 0x100000000 + lo };
}

export function encodeData(transactionId: number, data: Uint8Array): Uint8Array {
  const w = new ByteWriter();
  w.u32(transactionId);
  w.bytes(data);
  return encodePacket(PacketType.Data, w.toUint8Array());
}

export function encodeEndData(transactionId: number, data: Uint8Array): Uint8Array {
  const w = new ByteWriter();
  w.u32(transactionId);
  w.bytes(data);
  return encodePacket(PacketType.EndData, w.toUint8Array());
}

export function decodeDataPacket(payload: Uint8Array): { transactionId: number; data: Uint8Array } {
  const r = new ByteReader(payload);
  const transactionId = r.u32();
  const data = r.bytes(r.remaining);
  return { transactionId, data: new Uint8Array(data) };
}

export { DataPhase };
