// PTP/IP client: drives a command connection (and an event connection) over a
// pair of TCP sockets provided by the @aero-shutter/tcp-socket plugin.

import type { PluginListenerHandle } from '@capacitor/core';
import { TcpSocket } from '@aero-shutter/tcp-socket';
import { fromBase64, toBase64 } from '../base64';
import {
  DataType,
  OpCode,
  PTPIP_PORT,
  PacketType,
  PropCode,
  RespCode,
} from './constants';
import { DataPhase } from './packets';
import {
  decodeInitCommandAck,
  decodeDataPacket,
  decodeOperationResponse,
  decodePackets,
  decodeStartData,
  encodeEndData,
  encodeInitCommandRequest,
  encodeInitEventRequest,
  encodeOperationRequest,
  encodeStartData,
  type Packet,
} from './packets';
import { parseObjectInfo, type ObjectInfo } from './objectinfo';
import {
  decodePropValue,
  encodePropValue,
  parseDevicePropDesc,
  type DevicePropDesc,
} from './devprop';

export interface TransactionResult {
  response: { responseCode: number; params: number[] };
  data: Uint8Array;
}

interface PendingTransaction {
  transactionId: number;
  resolve: (r: TransactionResult) => void;
  reject: (e: Error) => void;
  dataChunks: Uint8Array[];
  totalLength: number;
  started: boolean;
}

const CHUNK_SIZE = 1024 * 1024; // 1 MiB partial-object window

function concat(chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((n, c) => n + c.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const c of chunks) {
    out.set(c, off);
    off += c.length;
  }
  return out;
}

function randomGuid(): Uint8Array {
  const g = new Uint8Array(16);
  crypto.getRandomValues(g);
  return g;
}

export class PtpIpClient {
  private cmdSocketId: string | null = null;
  private eventSocketId: string | null = null;
  private cmdBuffer: Uint8Array = new Uint8Array(0);
  private listeners: PluginListenerHandle[] = [];
  private txnCounter = 1;
  private pending: PendingTransaction | null = null;
  private sessionOpen = false;
  connectionNumber = 0;
  responderName = '';

  constructor(
    private host: string,
    private port: number = PTPIP_PORT,
    private hostName = 'AeroShutter',
  ) {}

  async connect(timeoutMs = 8000): Promise<void> {
    const guid = randomGuid();

    // Command connection.
    const cmd = await TcpSocket.connect({ host: this.host, port: this.port, timeoutMs });
    this.cmdSocketId = cmd.socketId;

    const dataHandle = await TcpSocket.addListener('data', (e) => {
      if (e.socketId === this.cmdSocketId) this.onCmdData(fromBase64(e.dataB64));
    });
    const errHandle = await TcpSocket.addListener('error', (e) => {
      if (e.socketId === this.cmdSocketId && this.pending) {
        this.pending.reject(new Error(e.message));
        this.pending = null;
      }
    });
    this.listeners.push(dataHandle, errHandle);

    const ack = await this.exchangeInit(guid);
    this.connectionNumber = ack.connectionNumber;
    this.responderName = ack.name;

    // Event connection (bound by connection number). Best-effort: some cameras
    // and the mock tolerate its absence, so failures here are non-fatal.
    try {
      const ev = await TcpSocket.connect({ host: this.host, port: this.port, timeoutMs });
      this.eventSocketId = ev.socketId;
      await TcpSocket.write({
        socketId: ev.socketId,
        dataB64: toBase64(encodeInitEventRequest(ack.connectionNumber)),
      });
    } catch {
      this.eventSocketId = null;
    }
  }

  private exchangeInit(guid: Uint8Array): Promise<{ connectionNumber: number; name: string }> {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('Init handshake timed out')), 8000);
      const handler = (data: Uint8Array) => {
        this.cmdBuffer = concat([this.cmdBuffer, data]);
        const { packets, consumed } = decodePackets(this.cmdBuffer);
        this.cmdBuffer = this.cmdBuffer.subarray(consumed);
        for (const p of packets) {
          if (p.type === PacketType.InitCommandAck) {
            clearTimeout(timer);
            this.tempInitHandler = null;
            const ack = decodeInitCommandAck(p.payload);
            resolve({ connectionNumber: ack.connectionNumber, name: ack.name });
            return;
          }
          if (p.type === PacketType.InitFail) {
            clearTimeout(timer);
            this.tempInitHandler = null;
            reject(new Error('Camera rejected the connection (InitFail)'));
            return;
          }
        }
      };
      this.tempInitHandler = handler;
      void TcpSocket.write({
        socketId: this.cmdSocketId!,
        dataB64: toBase64(encodeInitCommandRequest(guid, this.hostName)),
      });
    });
  }

  private tempInitHandler: ((data: Uint8Array) => void) | null = null;

  private onCmdData(data: Uint8Array): void {
    if (this.tempInitHandler) {
      this.tempInitHandler(data);
      return;
    }
    this.cmdBuffer = concat([this.cmdBuffer, data]);
    const { packets, consumed } = decodePackets(this.cmdBuffer);
    this.cmdBuffer = this.cmdBuffer.subarray(consumed);
    for (const p of packets) this.handlePacket(p);
  }

  private handlePacket(p: Packet): void {
    const t = this.pending;
    if (!t) return;
    switch (p.type) {
      case PacketType.StartData: {
        const sd = decodeStartData(p.payload);
        t.totalLength = sd.totalLength;
        t.started = true;
        break;
      }
      case PacketType.Data: {
        const { data } = decodeDataPacket(p.payload);
        t.dataChunks.push(data);
        break;
      }
      case PacketType.EndData: {
        const { data } = decodeDataPacket(p.payload);
        t.dataChunks.push(data);
        break;
      }
      case PacketType.OperationResponse: {
        const resp = decodeOperationResponse(p.payload);
        const result: TransactionResult = {
          response: { responseCode: resp.responseCode, params: resp.params },
          data: concat(t.dataChunks),
        };
        this.pending = null;
        t.resolve(result);
        break;
      }
      default:
        break;
    }
  }

  private transaction(
    opcode: number,
    params: number[] = [],
    dataPhase: number = DataPhase.DataToInitiator,
    outData?: Uint8Array,
  ): Promise<TransactionResult> {
    if (!this.cmdSocketId) return Promise.reject(new Error('Not connected'));
    return new Promise<TransactionResult>((resolve, reject) => {
      const transactionId = this.txnCounter++;
      this.pending = {
        transactionId,
        resolve,
        reject,
        dataChunks: [],
        totalLength: 0,
        started: false,
      };
      const timer = setTimeout(() => {
        if (this.pending?.transactionId === transactionId) {
          this.pending = null;
          reject(new Error(`Operation 0x${opcode.toString(16)} timed out`));
        }
      }, 15000);
      const origResolve = resolve;
      this.pending.resolve = (r) => {
        clearTimeout(timer);
        origResolve(r);
      };

      void (async () => {
        await TcpSocket.write({
          socketId: this.cmdSocketId!,
          dataB64: toBase64(encodeOperationRequest({ dataPhase, opcode, transactionId, params })),
        });
        if (dataPhase === DataPhase.DataToResponder && outData) {
          await TcpSocket.write({
            socketId: this.cmdSocketId!,
            dataB64: toBase64(encodeStartData(transactionId, outData.length)),
          });
          await TcpSocket.write({
            socketId: this.cmdSocketId!,
            dataB64: toBase64(encodeEndData(transactionId, outData)),
          });
        }
      })().catch((e) => {
        clearTimeout(timer);
        this.pending = null;
        reject(e instanceof Error ? e : new Error(String(e)));
      });
    });
  }

  private ensureOk(r: TransactionResult, op: string): TransactionResult {
    if (r.response.responseCode !== RespCode.OK) {
      throw new Error(`${op} failed: 0x${r.response.responseCode.toString(16)}`);
    }
    return r;
  }

  // ---- High-level operations ---------------------------------------------

  async openSession(): Promise<void> {
    if (this.sessionOpen) return;
    const r = await this.transaction(OpCode.OpenSession, [1], DataPhase.NoData);
    this.ensureOk(r, 'OpenSession');
    this.sessionOpen = true;
  }

  async closeSession(): Promise<void> {
    if (!this.sessionOpen) return;
    await this.transaction(OpCode.CloseSession, [], DataPhase.NoData).catch(() => undefined);
    this.sessionOpen = false;
  }

  async getStorageIds(): Promise<number[]> {
    const r = this.ensureOk(await this.transaction(OpCode.GetStorageIDs), 'GetStorageIDs');
    return readU32Array(r.data);
  }

  async getObjectHandles(storageId = 0xffffffff): Promise<number[]> {
    const r = this.ensureOk(
      await this.transaction(OpCode.GetObjectHandles, [storageId, 0, 0]),
      'GetObjectHandles',
    );
    return readU32Array(r.data);
  }

  async getObjectInfo(handle: number): Promise<ObjectInfo> {
    const r = this.ensureOk(
      await this.transaction(OpCode.GetObjectInfo, [handle]),
      'GetObjectInfo',
    );
    return parseObjectInfo(r.data);
  }

  async getThumb(handle: number): Promise<Uint8Array> {
    // Try Nikon's large thumbnail first, fall back to the standard thumb.
    try {
      const large = await this.transaction(OpCode.NikonGetLargeThumb, [handle]);
      if (large.response.responseCode === RespCode.OK && large.data.length > 0) {
        return large.data;
      }
    } catch {
      // fall through
    }
    const r = this.ensureOk(await this.transaction(OpCode.GetThumb, [handle]), 'GetThumb');
    return r.data;
  }

  async getObject(handle: number): Promise<Uint8Array> {
    const r = this.ensureOk(await this.transaction(OpCode.GetObject, [handle]), 'GetObject');
    return r.data;
  }

  /**
   * Stream an object in 1 MiB chunks via GetPartialObject, invoking onChunk for
   * each window. Never buffers the whole object.
   */
  async getObjectStreamed(
    handle: number,
    totalSize: number,
    onChunk: (chunk: Uint8Array, offset: number) => Promise<void>,
    startOffset = 0,
  ): Promise<void> {
    let offset = startOffset;
    while (offset < totalSize) {
      const want = Math.min(CHUNK_SIZE, totalSize - offset);
      const r = this.ensureOk(
        await this.transaction(OpCode.GetPartialObject, [handle, offset, want]),
        'GetPartialObject',
      );
      if (r.data.length === 0) break;
      await onChunk(r.data, offset);
      offset += r.data.length;
      if (r.data.length < want) break;
    }
  }

  async getDevicePropDesc(propCode: number): Promise<DevicePropDesc> {
    const r = this.ensureOk(
      await this.transaction(OpCode.GetDevicePropDesc, [propCode]),
      'GetDevicePropDesc',
    );
    return parseDevicePropDesc(r.data);
  }

  async getDevicePropValue(propCode: number, dataType: number): Promise<number> {
    const r = this.ensureOk(
      await this.transaction(OpCode.GetDevicePropValue, [propCode]),
      'GetDevicePropValue',
    );
    return decodePropValue(dataType, r.data);
  }

  async setDevicePropValue(propCode: number, dataType: number, value: number): Promise<void> {
    const payload = encodePropValue(dataType, value);
    const r = await this.transaction(
      OpCode.SetDevicePropValue,
      [propCode],
      DataPhase.DataToResponder,
      payload,
    );
    this.ensureOk(r, 'SetDevicePropValue');
  }

  async initiateCapture(): Promise<void> {
    const r = await this.transaction(OpCode.InitiateCapture, [0, 0], DataPhase.NoData);
    this.ensureOk(r, 'InitiateCapture');
  }

  async close(): Promise<void> {
    await this.closeSession();
    for (const l of this.listeners) await l.remove().catch(() => undefined);
    this.listeners = [];
    if (this.cmdSocketId) await TcpSocket.close({ socketId: this.cmdSocketId }).catch(() => undefined);
    if (this.eventSocketId) {
      await TcpSocket.close({ socketId: this.eventSocketId }).catch(() => undefined);
    }
    this.cmdSocketId = null;
    this.eventSocketId = null;
  }
}

function readU32Array(data: Uint8Array): number[] {
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  if (data.length < 4) return [];
  const count = view.getUint32(0, true);
  const out: number[] = [];
  for (let i = 0; i < count && 4 + i * 4 + 4 <= data.length; i++) {
    out.push(view.getUint32(4 + i * 4, true));
  }
  return out;
}

export { PropCode, DataType };
