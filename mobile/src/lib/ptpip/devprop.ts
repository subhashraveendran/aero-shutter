// DevicePropDesc parsing + property value (de)serialization + formatting.

import { ByteReader, ByteWriter } from './buffer';
import { DataType, PropCode } from './constants';

export type PropFormFlag = 0x00 | 0x01 | 0x02; // none, range, enum

export interface DevicePropDesc {
  propCode: number;
  dataType: number;
  getSet: number; // 0 = read-only, 1 = read-write
  factoryDefault: number;
  currentValue: number;
  formFlag: PropFormFlag;
  range?: { min: number; max: number; step: number };
  enumValues?: number[];
}

function readTypedValue(r: ByteReader, dataType: number): number {
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
      // Unknown / unsupported width; consume nothing meaningful.
      return r.u16();
  }
}

export function writeTypedValue(w: ByteWriter, dataType: number, value: number): void {
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

export function parseDevicePropDesc(payload: Uint8Array): DevicePropDesc {
  const r = new ByteReader(payload);
  const propCode = r.u16();
  const dataType = r.u16();
  const getSet = r.u8();
  const factoryDefault = readTypedValue(r, dataType);
  const currentValue = readTypedValue(r, dataType);
  const formFlag = r.u8() as PropFormFlag;

  const desc: DevicePropDesc = {
    propCode,
    dataType,
    getSet,
    factoryDefault,
    currentValue,
    formFlag,
  };

  if (formFlag === 0x01) {
    desc.range = {
      min: readTypedValue(r, dataType),
      max: readTypedValue(r, dataType),
      step: readTypedValue(r, dataType),
    };
  } else if (formFlag === 0x02) {
    const count = r.u16();
    const values: number[] = [];
    for (let i = 0; i < count; i++) values.push(readTypedValue(r, dataType));
    desc.enumValues = values;
  }

  return desc;
}

export function encodeDevicePropDesc(desc: DevicePropDesc): Uint8Array {
  const w = new ByteWriter();
  w.u16(desc.propCode);
  w.u16(desc.dataType);
  w.u8(desc.getSet);
  writeTypedValue(w, desc.dataType, desc.factoryDefault);
  writeTypedValue(w, desc.dataType, desc.currentValue);
  w.u8(desc.formFlag);
  if (desc.formFlag === 0x01 && desc.range) {
    writeTypedValue(w, desc.dataType, desc.range.min);
    writeTypedValue(w, desc.dataType, desc.range.max);
    writeTypedValue(w, desc.dataType, desc.range.step);
  } else if (desc.formFlag === 0x02 && desc.enumValues) {
    w.u16(desc.enumValues.length);
    for (const v of desc.enumValues) writeTypedValue(w, desc.dataType, v);
  }
  return w.toUint8Array();
}

export function encodePropValue(dataType: number, value: number): Uint8Array {
  const w = new ByteWriter();
  writeTypedValue(w, dataType, value);
  return w.toUint8Array();
}

export function decodePropValue(dataType: number, payload: Uint8Array): number {
  return readTypedValue(new ByteReader(payload), dataType);
}

// ---- Human-readable formatting -------------------------------------------

const WHITE_BALANCE: Record<number, string> = {
  1: 'Manual',
  2: 'Auto',
  3: 'One-push Auto',
  4: 'Daylight',
  5: 'Fluorescent',
  6: 'Tungsten',
  7: 'Flash',
  32784: 'Cloudy',
  32785: 'Shade',
  32786: 'Color Temp.',
  32787: 'Preset',
};

const EXPOSURE_PROGRAM: Record<number, string> = {
  1: 'Manual',
  2: 'Program',
  3: 'Aperture Priority',
  4: 'Shutter Priority',
};

export function formatFNumber(raw: number): string {
  return `f/${(raw / 100).toFixed(raw % 100 === 0 ? 0 : 1)}`;
}

export function formatExposureTime(raw: number): string {
  // raw is in units of 0.1 ms.
  const seconds = raw / 10000;
  if (seconds <= 0) return '—';
  if (seconds >= 1) {
    return `${seconds % 1 === 0 ? seconds.toFixed(0) : seconds.toFixed(1)}s`;
  }
  const denom = Math.round(1 / seconds);
  return `1/${denom}s`;
}

export function formatExposureBias(raw: number): string {
  // raw is in millistops (thousandths of a stop) per PTP EV compensation.
  const ev = raw / 1000;
  const sign = ev > 0 ? '+' : ev < 0 ? '' : '';
  if (ev === 0) return '0 EV';
  return `${sign}${ev.toFixed(1)} EV`;
}

export function formatIso(raw: number): string {
  return `ISO ${raw}`;
}

export function formatWhiteBalance(raw: number): string {
  return WHITE_BALANCE[raw] ?? `WB ${raw}`;
}

export function formatExposureProgram(raw: number): string {
  return EXPOSURE_PROGRAM[raw] ?? `Mode ${raw}`;
}

/** Format any known property value into a display string. */
export function formatPropValue(propCode: number, value: number): string {
  switch (propCode) {
    case PropCode.FNumber:
      return formatFNumber(value);
    case PropCode.ExposureTime:
      return formatExposureTime(value);
    case PropCode.ExposureBiasCompensation:
      return formatExposureBias(value);
    case PropCode.ExposureIndex:
      return formatIso(value);
    case PropCode.WhiteBalance:
      return formatWhiteBalance(value);
    case PropCode.ExposureProgramMode:
      return formatExposureProgram(value);
    case PropCode.BatteryLevel:
      return `${value}%`;
    default:
      return String(value);
  }
}

export function propLabel(propCode: number): string {
  switch (propCode) {
    case PropCode.FNumber:
      return 'Aperture';
    case PropCode.ExposureTime:
      return 'Shutter';
    case PropCode.ExposureBiasCompensation:
      return 'Exposure Comp.';
    case PropCode.ExposureIndex:
      return 'ISO';
    case PropCode.WhiteBalance:
      return 'White Balance';
    case PropCode.ExposureProgramMode:
      return 'Mode';
    case PropCode.BatteryLevel:
      return 'Battery';
    default:
      return `Prop 0x${propCode.toString(16)}`;
  }
}
