// Little-endian binary reader/writer helpers plus PTP string & datetime codecs.

export class ByteWriter {
  private chunks: number[] = [];

  u8(v: number): this {
    this.chunks.push(v & 0xff);
    return this;
  }

  u16(v: number): this {
    this.chunks.push(v & 0xff, (v >>> 8) & 0xff);
    return this;
  }

  u32(v: number): this {
    this.chunks.push(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff);
    return this;
  }

  i8(v: number): this {
    return this.u8(v < 0 ? v + 0x100 : v);
  }

  i16(v: number): this {
    return this.u16(v < 0 ? v + 0x10000 : v);
  }

  i32(v: number): this {
    return this.u32(v < 0 ? v + 0x100000000 : v);
  }

  bytes(data: Uint8Array): this {
    for (let i = 0; i < data.length; i++) this.chunks.push(data[i]);
    return this;
  }

  /** PTP string: u8 length (chars incl. NUL, 0 if empty) + UTF-16LE + NUL terminator. */
  ptpString(s: string): this {
    if (s.length === 0) {
      this.u8(0);
      return this;
    }
    const chars = [...s];
    this.u8(chars.length + 1);
    for (const ch of chars) {
      const code = ch.codePointAt(0) ?? 0;
      this.u16(code);
    }
    this.u16(0); // NUL terminator
    return this;
  }

  toUint8Array(): Uint8Array {
    return new Uint8Array(this.chunks);
  }
}

export class ByteReader {
  private view: DataView;
  offset = 0;

  constructor(private data: Uint8Array) {
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  get remaining(): number {
    return this.data.length - this.offset;
  }

  u8(): number {
    const v = this.view.getUint8(this.offset);
    this.offset += 1;
    return v;
  }

  u16(): number {
    const v = this.view.getUint16(this.offset, true);
    this.offset += 2;
    return v;
  }

  u32(): number {
    const v = this.view.getUint32(this.offset, true);
    this.offset += 4;
    return v;
  }

  i8(): number {
    const v = this.view.getInt8(this.offset);
    this.offset += 1;
    return v;
  }

  i16(): number {
    const v = this.view.getInt16(this.offset, true);
    this.offset += 2;
    return v;
  }

  i32(): number {
    const v = this.view.getInt32(this.offset, true);
    this.offset += 4;
    return v;
  }

  u64(): bigint {
    const v = this.view.getBigUint64(this.offset, true);
    this.offset += 8;
    return v;
  }

  bytes(n: number): Uint8Array {
    const out = this.data.subarray(this.offset, this.offset + n);
    this.offset += n;
    return out;
  }

  /** PTP string: u8 length (chars incl. NUL) + UTF-16LE. Returns without terminator. */
  ptpString(): string {
    const len = this.u8();
    if (len === 0) return '';
    let out = '';
    for (let i = 0; i < len; i++) {
      const code = this.u16();
      if (code !== 0) out += String.fromCodePoint(code);
    }
    return out;
  }
}

/**
 * Parse a PTP datetime string ("YYYYMMDDThhmmss" optionally with ".s" and tz).
 * Returns epoch millis, or null if unparseable.
 */
export function parsePtpDateTime(s: string): number | null {
  const m = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})/.exec(s);
  if (!m) return null;
  const [, y, mo, d, h, mi, se] = m;
  const dt = new Date(
    Number(y),
    Number(mo) - 1,
    Number(d),
    Number(h),
    Number(mi),
    Number(se),
  );
  const t = dt.getTime();
  return Number.isNaN(t) ? null : t;
}

/** Format epoch millis as a PTP datetime string. */
export function formatPtpDateTime(epochMs: number): string {
  const d = new Date(epochMs);
  const p = (n: number, w = 2) => String(n).padStart(w, '0');
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}T${p(d.getHours())}${p(
    d.getMinutes(),
  )}${p(d.getSeconds())}`;
}
