import { describe, expect, it } from 'vitest';
import {
  decodeInitCommandAck,
  decodeOperationRequest,
  decodeOperationResponse,
  decodePackets,
  decodeStartData,
  encodeInitCommandRequest,
  encodeOperationRequest,
  encodeOperationResponse,
  encodePacket,
  encodeStartData,
} from './packets';
import { PacketType } from './constants';
import { DataPhase } from './packets';

describe('packet framing', () => {
  it('round-trips a generic packet', () => {
    const payload = new Uint8Array([1, 2, 3, 4, 5]);
    const framed = encodePacket(PacketType.Event, payload);
    const { packets, consumed } = decodePackets(framed);
    expect(consumed).toBe(framed.length);
    expect(packets).toHaveLength(1);
    expect(packets[0].type).toBe(PacketType.Event);
    expect([...packets[0].payload]).toEqual([1, 2, 3, 4, 5]);
  });

  it('decodes multiple concatenated packets and leaves partial trailing bytes', () => {
    const a = encodePacket(PacketType.ProbeRequest, new Uint8Array([9]));
    const b = encodePacket(PacketType.ProbeResponse, new Uint8Array([8, 7]));
    const merged = new Uint8Array(a.length + b.length + 3);
    merged.set(a, 0);
    merged.set(b, a.length);
    merged.set([0xaa, 0xbb, 0xcc], a.length + b.length); // partial header
    const { packets, consumed } = decodePackets(merged);
    expect(packets).toHaveLength(2);
    expect(consumed).toBe(a.length + b.length);
  });

  it('round-trips OperationRequest', () => {
    const req = {
      dataPhase: DataPhase.DataToInitiator,
      opcode: 0x1007,
      transactionId: 42,
      params: [0xffffffff, 0, 0],
    };
    const framed = encodeOperationRequest(req);
    const { packets } = decodePackets(framed);
    const decoded = decodeOperationRequest(packets[0].payload);
    expect(decoded).toEqual(req);
  });

  it('round-trips OperationResponse', () => {
    const framed = encodeOperationResponse({
      responseCode: 0x2001,
      transactionId: 7,
      params: [123],
    });
    const { packets } = decodePackets(framed);
    const decoded = decodeOperationResponse(packets[0].payload);
    expect(decoded.responseCode).toBe(0x2001);
    expect(decoded.transactionId).toBe(7);
    expect(decoded.params).toEqual([123]);
  });

  it('round-trips StartData with a large u64 length', () => {
    const framed = encodeStartData(5, 5_000_000_000);
    const { packets } = decodePackets(framed);
    const sd = decodeStartData(packets[0].payload);
    expect(sd.transactionId).toBe(5);
    expect(sd.totalLength).toBe(5_000_000_000);
  });

  it('round-trips InitCommandRequest / Ack shape', () => {
    const guid = new Uint8Array(16).map((_, i) => i);
    const framed = encodeInitCommandRequest(guid, 'AeroShutter');
    const { packets } = decodePackets(framed);
    expect(packets[0].type).toBe(PacketType.InitCommandRequest);

    // Build a matching ack payload and decode it.
    const ackPayload = new Uint8Array(4 + 16 + 2);
    new DataView(ackPayload.buffer).setUint32(0, 3, true);
    for (let i = 0; i < 16; i++) ackPayload[4 + i] = i + 1;
    const ack = decodeInitCommandAck(ackPayload);
    expect(ack.connectionNumber).toBe(3);
    expect(ack.guid).toHaveLength(16);
  });
});
