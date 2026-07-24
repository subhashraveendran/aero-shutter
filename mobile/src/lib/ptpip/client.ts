// PTP/IP client: drives a command connection (and an event connection) over a
// pair of TCP sockets provided by the @aero-shutter/tcp-socket plugin.

import { Capacitor, type PluginListenerHandle } from '@capacitor/core';
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
  decodeEvent,
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
  type EventPacket,
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

/** Callback invoked for each decoded PTP/IP Event packet on the event channel. */
export type EventHandler = (event: EventPacket) => void;

/** Callback invoked exactly once when the connection is detected as dead. */
export type DisconnectHandler = (reason: string) => void;

interface PendingTransaction {
  transactionId: number;
  resolve: (r: TransactionResult) => void;
  reject: (e: Error) => void;
  dataChunks: Uint8Array[];
  totalLength: number;
  started: boolean;
}

const CHUNK_SIZE = 4 * 1024 * 1024; // 4 MiB partial-object window: fewer round-trips = higher Wi-Fi throughput

// WMU_INITIATOR_GUID is the fixed 16-byte initiator GUID that Nikon's Wireless
// Mobile Utility hardcodes (00112233445566778899aabbccddeeff). Sending a stable
// GUID on every connection — rather than a random one — lets the camera treat
// us as the same returning host and skip re-pairing prompts.
const WMU_INITIATOR_GUID = new Uint8Array([
  0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
]);

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

function initiatorGuid(): Uint8Array {
  // Fixed WMU GUID (see WMU_INITIATOR_GUID). Copied so the shared constant is
  // never mutated by downstream encoders.
  return WMU_INITIATOR_GUID.slice();
}

// defaultHostName builds the PTP/IP friendly name in the shape Nikon's Wireless
// Mobile Utility uses ("App/Version (OS x)"), e.g. "aero-shutter/1.0.0 (android)".
export function defaultHostName(): string {
  const version = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0';
  const platform = Capacitor.getPlatform();
  return `aero-shutter/${version} (${platform})`;
}

export class PtpIpClient {
  private cmdSocketId: string | null = null;
  private eventSocketId: string | null = null;
  private cmdBuffer: Uint8Array = new Uint8Array(0);
  private listeners: PluginListenerHandle[] = [];
  private txnCounter = 1;
  private pending: PendingTransaction | null = null;
  private sessionOpen = false;
  // Serialization lock. PTP/IP on a single command connection is strictly
  // request -> response: only ONE transaction may be in flight at a time.
  // The app fires overlapping operations (listing getObjectInfo, thumbnail
  // getThumb, keep-alive, capture, ...) which would otherwise clobber the
  // single `pending` slot and desync the connection. We chain every
  // transaction onto this promise so a new one never starts until the previous
  // one has fully settled (response, timeout, or error) and `pending` is clear.
  private txnQueue: Promise<unknown> = Promise.resolve();
  // Registered callbacks (fix #2 event dispatch, fix #4 disconnect detection).
  private eventHandler: EventHandler | null = null;
  private disconnectHandler: DisconnectHandler | null = null;
  // Guards the disconnect callback so it fires at most once per connection.
  private disconnected = false;
  connectionNumber = 0;
  responderName = '';
  /** How the underlying camera socket was bound to the network. */
  networkBinding: string | null = null;

  constructor(
    private host: string,
    private port: number = PTPIP_PORT,
    private hostName = defaultHostName(),
    private bindWifi = true,
  ) {}

  /**
   * Register a handler for post-handshake PTP/IP Event packets (fix #2).
   * Replaces any previously-registered handler. Pass null to clear.
   */
  setEventHandler(cb: EventHandler | null): void {
    this.eventHandler = cb;
  }

  /**
   * Register a handler fired exactly once when the connection is detected dead
   * (event-socket error/close, or a transaction failing on a dead socket).
   * Fix #4. Pass null to clear.
   */
  setDisconnectHandler(cb: DisconnectHandler | null): void {
    this.disconnectHandler = cb;
  }

  /** Fire the disconnect callback at most once for this connection. */
  private fireDisconnect(reason: string): void {
    if (this.disconnected) return;
    this.disconnected = true;
    const cb = this.disconnectHandler;
    if (cb) {
      try {
        cb(reason);
      } catch {
        /* handler errors are non-fatal */
      }
    }
  }

  async connect(timeoutMs = 8000): Promise<void> {
    const guid = initiatorGuid();
    this.disconnected = false;

    // Command connection.
    const cmd = await TcpSocket.connect({
      host: this.host,
      port: this.port,
      timeoutMs,
      bindWifi: this.bindWifi,
    });
    this.cmdSocketId = cmd.socketId;
    this.networkBinding = cmd.networkBinding ?? null;

    const dataHandle = await TcpSocket.addListener('data', (e) => {
      if (e.socketId === this.cmdSocketId) this.onCmdData(fromBase64(e.dataB64));
    });
    const errHandle = await TcpSocket.addListener('error', (e) => {
      if (e.socketId === this.cmdSocketId) {
        if (this.pending) {
          this.pending.reject(new Error(e.message));
          this.pending = null;
        }
        // A command-socket error means the link is dead (fix #4).
        this.fireDisconnect(e.message || 'Command socket error');
      }
    });
    this.listeners.push(dataHandle, errHandle);

    const ack = await this.exchangeInit(guid);
    this.connectionNumber = ack.connectionNumber;
    this.responderName = ack.name;

    // Event connection (bound by the connection number from the command ack).
    // This is REQUIRED: Nikon bodies (e.g. the D5300) will not answer
    // OpenSession on the command channel until the event channel handshake has
    // completed, so we must send InitEventRequest AND wait for InitEventAck
    // before proceeding — otherwise OpenSession (0x1002) times out.
    const ev = await TcpSocket.connect({
      host: this.host,
      port: this.port,
      timeoutMs,
      bindWifi: this.bindWifi,
    });
    this.eventSocketId = ev.socketId;
    const evHandle = await TcpSocket.addListener('data', (e) => {
      if (e.socketId === this.eventSocketId) this.onEventData(fromBase64(e.dataB64));
    });
    // Event-socket error/close is our earliest signal the camera went away —
    // fire the disconnect callback immediately rather than waiting for the next
    // user operation to fail (fix #4).
    const evErrHandle = await TcpSocket.addListener('error', (e) => {
      if (e.socketId === this.eventSocketId) {
        if (this.eventInitReject) {
          const fail = this.eventInitReject;
          this.eventInitResolve = null;
          this.eventInitReject = null;
          fail(new Error(e.message || 'Event socket error'));
        }
        this.fireDisconnect(e.message || 'Event socket closed');
      }
    });
    this.listeners.push(evHandle, evErrHandle);

    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.eventInitResolve = null;
        this.eventInitReject = null;
        reject(new Error('Event channel handshake timed out'));
      }, timeoutMs);
      this.eventInitResolve = () => {
        clearTimeout(timer);
        resolve();
      };
      this.eventInitReject = (err) => {
        clearTimeout(timer);
        reject(err);
      };
      void TcpSocket.write({
        socketId: ev.socketId,
        dataB64: toBase64(encodeInitEventRequest(ack.connectionNumber)),
      }).catch((err) => {
        clearTimeout(timer);
        this.eventInitResolve = null;
        this.eventInitReject = null;
        reject(err instanceof Error ? err : new Error(String(err)));
      });
    });
  }

  private eventBuffer: Uint8Array = new Uint8Array(0);
  private eventInitResolve: (() => void) | null = null;
  private eventInitReject: ((e: Error) => void) | null = null;

  // onEventData drives the event-channel handshake (resolving once InitEventAck
  // arrives) and thereafter decodes each Event packet and dispatches it to the
  // registered event handler (fix #2). The buffer is still drained so the camera
  // is never blocked writing to us.
  private onEventData(data: Uint8Array): void {
    this.eventBuffer = concat([this.eventBuffer, data]);
    const { packets, consumed } = decodePackets(this.eventBuffer);
    this.eventBuffer = this.eventBuffer.subarray(consumed);
    for (const p of packets) {
      if (p.type === PacketType.InitEventAck && this.eventInitResolve) {
        const done = this.eventInitResolve;
        this.eventInitResolve = null;
        this.eventInitReject = null;
        done();
      } else if (p.type === PacketType.InitFail && this.eventInitReject) {
        const fail = this.eventInitReject;
        this.eventInitResolve = null;
        this.eventInitReject = null;
        fail(new Error('Camera rejected the event connection (InitFail)'));
      } else if (p.type === PacketType.Event) {
        // Decode + dispatch post-handshake events (e.g. ObjectAdded) to the
        // registered handler. Buffer is already drained above for back-compat.
        const event = decodeEvent(p.payload);
        const handler = this.eventHandler;
        if (handler) {
          try {
            handler(event);
          } catch {
            /* handler errors are non-fatal */
          }
        }
      }
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

  /**
   * Public entry point for a PTP transaction. Serialized: the actual exchange
   * (runTransaction) is chained onto txnQueue so it only starts once the
   * previous transaction has settled. A failed/timed-out transaction still
   * releases the lock (the chain uses a settle-catch) so the queue continues.
   */
  private transaction(
    opcode: number,
    params: number[] = [],
    dataPhase: number = DataPhase.DataToInitiator,
    outData?: Uint8Array,
  ): Promise<TransactionResult> {
    const run = async (): Promise<TransactionResult> => {
      let result: TransactionResult;
      try {
        result = await this.runTransaction(opcode, params, dataPhase, outData);
      } catch (e) {
        // A transaction failing on a dead socket means the link is gone: fire
        // the disconnect callback immediately (fix #4). OpenSession/CloseSession
        // guard against re-entrancy loops from the recovery path below.
        this.fireDisconnect(e instanceof Error ? e.message : String(e));
        throw e;
      }
      // Fix #10: if the responder reports "Session Not Open", transparently
      // re-open the session and retry the transaction once before failing.
      if (
        result.response.responseCode === RespCode.SessionNotOpen &&
        opcode !== OpCode.OpenSession &&
        opcode !== OpCode.CloseSession
      ) {
        this.sessionOpen = false;
        try {
          await this.reopenSession();
        } catch {
          return result; // couldn't recover; surface the original response
        }
        return this.runTransaction(opcode, params, dataPhase, outData);
      }
      return result;
    };
    // Wait for the prior transaction to fully settle (ignore its outcome), then
    // run ours. The returned promise reflects ONLY our transaction's result.
    const result = this.txnQueue.then(run, run);
    // Keep the chain alive regardless of this transaction's success/failure so
    // a rejection never breaks the queue for subsequent callers.
    this.txnQueue = result.then(
      () => undefined,
      () => undefined,
    );
    return result;
  }

  private runTransaction(
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

  /**
   * Re-open the PTP session from WITHIN a serialized transaction run (fix #10).
   * Must call runTransaction directly — going back through transaction() would
   * deadlock on the txnQueue we are already holding.
   */
  private async reopenSession(): Promise<void> {
    const r = await this.runTransaction(OpCode.OpenSession, [1], DataPhase.NoData);
    if (r.response.responseCode !== RespCode.OK) {
      throw new Error(`Session re-open failed: 0x${r.response.responseCode.toString(16)}`);
    }
    this.sessionOpen = true;
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

  /**
   * Cheap serialized round-trip used as a keep-alive so the camera doesn't drop
   * an idle PTP session / Wi-Fi link. GetStorageIDs is tiny and always
   * supported; it goes through the same mutex as every other transaction so it
   * never overlaps real work. Throws on a dead link so the caller can react.
   */
  async keepAlive(): Promise<void> {
    this.ensureOk(await this.transaction(OpCode.GetStorageIDs), 'KeepAlive');
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
   * Lock (mode=TransferListLock) or unlock (mode=TransferListUnlock) the camera's
   * "Send to smart device" transfer queue via Nikon vendor op 0x9407, so the app
   * can read the marked list without the camera mutating it. Throws on bodies
   * that don't implement the vendor op (caller should degrade gracefully).
   */
  async setTransferListLock(mode: number): Promise<void> {
    this.ensureOk(
      await this.transaction(OpCode.NikonSetTransferListLock, [mode]),
      'SetTransferListLock',
    );
  }

  /**
   * Return the object handles the user marked on the camera body for "Send to
   * smart device" via Nikon vendor op 0x9408 (AUINT32 handle array). Throws on
   * unsupported bodies.
   */
  async getTransferList(): Promise<number[]> {
    const r = this.ensureOk(
      await this.transaction(OpCode.NikonGetTransferList),
      'GetTransferList',
    );
    return r.data.length === 0 ? [] : readU32Array(r.data);
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
    // Suppress the disconnect callback for an explicit, user-initiated close.
    this.disconnected = true;
    this.eventHandler = null;
    this.disconnectHandler = null;
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
