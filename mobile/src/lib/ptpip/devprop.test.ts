import { describe, expect, it } from 'vitest';
import {
  encodeDevicePropDesc,
  formatExposureBias,
  formatExposureTime,
  formatFNumber,
  formatIso,
  formatPropValue,
  formatWhiteBalance,
  parseDevicePropDesc,
  type DevicePropDesc,
} from './devprop';
import { DataType, PropCode } from './constants';

describe('property formatting', () => {
  it('formats FNumber (u16/100)', () => {
    expect(formatFNumber(560)).toBe('f/5.6');
    expect(formatFNumber(800)).toBe('f/8');
  });

  it('formats ExposureTime (0.1ms units)', () => {
    expect(formatExposureTime(40)).toBe('1/250s');
    expect(formatExposureTime(10000)).toBe('1s');
    expect(formatExposureTime(20000)).toBe('2s');
  });

  it('formats exposure bias in millistops', () => {
    expect(formatExposureBias(700)).toBe('+0.7 EV');
    expect(formatExposureBias(-1000)).toBe('-1.0 EV');
    expect(formatExposureBias(0)).toBe('0 EV');
  });

  it('formats ISO and white balance including Nikon vendor codes', () => {
    expect(formatIso(400)).toBe('ISO 400');
    expect(formatWhiteBalance(2)).toBe('Auto');
    expect(formatWhiteBalance(32784)).toBe('Cloudy');
    expect(formatWhiteBalance(32787)).toBe('Preset');
  });

  it('dispatches formatPropValue by prop code', () => {
    expect(formatPropValue(PropCode.FNumber, 560)).toBe('f/5.6');
    expect(formatPropValue(PropCode.ExposureIndex, 800)).toBe('ISO 800');
  });
});

describe('DevicePropDesc parsing', () => {
  it('round-trips an enum-form u16 property', () => {
    const desc: DevicePropDesc = {
      propCode: PropCode.ExposureIndex,
      dataType: DataType.UINT16,
      getSet: 1,
      factoryDefault: 100,
      currentValue: 400,
      formFlag: 0x02,
      enumValues: [100, 200, 400, 800],
    };
    const decoded = parseDevicePropDesc(encodeDevicePropDesc(desc));
    expect(decoded).toEqual(desc);
  });

  it('round-trips a range-form int16 property', () => {
    const desc: DevicePropDesc = {
      propCode: PropCode.ExposureBiasCompensation,
      dataType: DataType.INT16,
      getSet: 1,
      factoryDefault: 0,
      currentValue: -700,
      formFlag: 0x01,
      range: { min: -3000, max: 3000, step: 333 },
    };
    const decoded = parseDevicePropDesc(encodeDevicePropDesc(desc));
    expect(decoded.currentValue).toBe(-700);
    expect(decoded.range).toEqual({ min: -3000, max: 3000, step: 333 });
  });
});
