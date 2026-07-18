import { describe, expect, it } from 'vitest';
import { compareVersions, isNewer, parseVersion, satisfiesMin } from './semver';

describe('semver util', () => {
  it('parses versions and ignores v-prefix and suffixes', () => {
    expect(parseVersion('1.2.3')).toEqual([1, 2, 3]);
    expect(parseVersion('v0.7.0')).toEqual([0, 7, 0]);
    expect(parseVersion('0.0.0-web')).toEqual([0, 0, 0]);
    expect(parseVersion('2.1')).toEqual([2, 1, 0]);
    expect(parseVersion('garbage')).toEqual([0, 0, 0]);
  });

  it('compares versions correctly', () => {
    expect(compareVersions('1.0.0', '1.0.0')).toBe(0);
    expect(compareVersions('1.0.1', '1.0.0')).toBe(1);
    expect(compareVersions('1.0.0', '1.0.1')).toBe(-1);
    expect(compareVersions('0.8.0', '0.7.9')).toBe(1);
    expect(compareVersions('v2.0.0', '1.9.9')).toBe(1);
  });

  it('isNewer is strict', () => {
    expect(isNewer('0.8.0', '0.7.0')).toBe(true);
    expect(isNewer('0.7.0', '0.7.0')).toBe(false);
    expect(isNewer('0.6.0', '0.7.0')).toBe(false);
  });

  it('satisfiesMin is >=', () => {
    expect(satisfiesMin('0.7.0', '0.7.0')).toBe(true);
    expect(satisfiesMin('0.8.0', '0.7.0')).toBe(true);
    expect(satisfiesMin('0.6.0', '0.7.0')).toBe(false);
  });
});
