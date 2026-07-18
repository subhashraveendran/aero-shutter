import { afterEach, describe, expect, it, vi } from 'vitest';

// Control demo mode + expose a stub plugin so we can exercise both the demo
// simulation and the native-passthrough branches of the wifi wrapper.
let mockDemo = false;
const pluginStub: Record<string, unknown> = {};

vi.mock('@aero-shutter/tcp-socket', () => ({
  isDemoMode: () => mockDemo,
  get TcpSocket() {
    return pluginStub;
  },
}));

import {
  currentWifi,
  findSsidByPrefix,
  isWifiJoinSupported,
  joinWifi,
  matchSsidByPrefix,
  NIKON_SSID_PREFIX,
} from './wifi';

afterEach(() => {
  mockDemo = false;
  for (const k of Object.keys(pluginStub)) delete pluginStub[k];
  vi.clearAllMocks();
});

describe('matchSsidByPrefix', () => {
  it('finds the first SSID with the prefix, case-insensitively', () => {
    const nets = [{ ssid: 'HomeWifi' }, { ssid: 'Nikon_WU2_ABCDEF' }, { ssid: 'Nikon_WU2_2' }];
    expect(matchSsidByPrefix(nets, 'Nikon_WU2_')).toBe('Nikon_WU2_ABCDEF');
    expect(matchSsidByPrefix(nets, 'nikon_wu2_')).toBe('Nikon_WU2_ABCDEF');
  });

  it('accepts a plain string array', () => {
    expect(matchSsidByPrefix(['a', 'Nikon_WU2_9'], 'Nikon_WU2_')).toBe('Nikon_WU2_9');
  });

  it('returns null when nothing matches or the list is empty', () => {
    expect(matchSsidByPrefix([{ ssid: 'HomeWifi' }], 'Nikon_WU2_')).toBeNull();
    expect(matchSsidByPrefix([], 'Nikon_WU2_')).toBeNull();
  });
});

describe('joinWifi (demo mode)', () => {
  it('returns a simulated success with a Nikon-prefixed SSID', async () => {
    mockDemo = true;
    const res = await joinWifi({ ssidPrefix: NIKON_SSID_PREFIX });
    expect(res.joined).toBe(true);
    expect(res.bound).toBe(false);
    expect(res.ssid.startsWith(NIKON_SSID_PREFIX)).toBe(true);
  });

  it('honors an explicit SSID in demo mode', async () => {
    mockDemo = true;
    const res = await joinWifi({ ssid: 'Nikon_WU2_1234' });
    expect(res).toEqual({ joined: true, ssid: 'Nikon_WU2_1234', bound: false });
  });

  it('reports support in demo mode even without a native plugin', () => {
    mockDemo = true;
    expect(isWifiJoinSupported()).toBe(true);
  });
});

describe('joinWifi (native passthrough)', () => {
  it('delegates to the plugin and returns its result', async () => {
    const spy = vi.fn(async () => ({ joined: true, ssid: 'Nikon_WU2_XY', bound: true }));
    pluginStub.joinWifi = spy;
    const res = await joinWifi({ ssid: 'Nikon_WU2_XY' });
    expect(spy).toHaveBeenCalledWith({ ssid: 'Nikon_WU2_XY' });
    expect(res.bound).toBe(true);
  });

  it('reports joined:false when the plugin method is missing', async () => {
    const res = await joinWifi({ ssidPrefix: NIKON_SSID_PREFIX });
    expect(res.joined).toBe(false);
    expect(isWifiJoinSupported()).toBe(false);
  });

  it('swallows plugin errors and returns joined:false', async () => {
    pluginStub.joinWifi = vi.fn(async () => {
      throw new Error('user cancelled');
    });
    const res = await joinWifi({ ssid: 'Nikon_WU2_XY' });
    expect(res.joined).toBe(false);
    expect(res.ssid).toBe('Nikon_WU2_XY');
  });
});

describe('currentWifi / findSsidByPrefix', () => {
  it('returns the plugin SSID, or null when absent', async () => {
    expect(await currentWifi()).toBeNull();
    pluginStub.currentWifi = vi.fn(async () => ({ ssid: 'Nikon_WU2_Z' }));
    expect(await currentWifi()).toBe('Nikon_WU2_Z');
  });

  it('scans and matches a prefix via the plugin', async () => {
    pluginStub.scanWifi = vi.fn(async () => ({
      networks: [{ ssid: 'Home' }, { ssid: 'Nikon_WU2_AA' }],
    }));
    expect(await findSsidByPrefix('Nikon_WU2_')).toBe('Nikon_WU2_AA');
  });

  it('returns null from findSsidByPrefix when scan is unavailable', async () => {
    expect(await findSsidByPrefix('Nikon_WU2_')).toBeNull();
  });
});
