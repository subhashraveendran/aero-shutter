import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@aero-shutter/tcp-socket', () => {
  return {
    isDemoMode: () => mockDemo,
    TcpSocket: {
      getWifiInfo: () => Promise.resolve(mockWifiInfo),
    },
  };
});

let mockDemo = false;
let mockWifiInfo: { gateway?: string | null; ipAddress?: string | null } = {};

import {
  candidateHosts,
  DEMO_HOST,
  gatewayGuessFromIp,
  NIKON_AP_CANDIDATES,
} from './discovery';

afterEach(() => {
  mockDemo = false;
  mockWifiInfo = {};
});

describe('gatewayGuessFromIp', () => {
  it('derives the /24 .1 gateway from an interface IP', () => {
    expect(gatewayGuessFromIp('192.168.1.57')).toBe('192.168.1.1');
    expect(gatewayGuessFromIp('10.0.5.99')).toBe('10.0.5.1');
  });

  it('rejects loopback, link-local and malformed input', () => {
    expect(gatewayGuessFromIp('127.0.0.1')).toBeNull();
    expect(gatewayGuessFromIp('169.254.1.5')).toBeNull();
    expect(gatewayGuessFromIp('0.0.0.0')).toBeNull();
    expect(gatewayGuessFromIp('not-an-ip')).toBeNull();
    expect(gatewayGuessFromIp('999.1.1.1')).toBeNull();
    expect(gatewayGuessFromIp(null)).toBeNull();
    expect(gatewayGuessFromIp(undefined)).toBeNull();
  });
});

describe('candidateHosts', () => {
  it('returns just the demo host in demo mode', async () => {
    mockDemo = true;
    const hosts = await candidateHosts('192.168.1.1');
    expect(hosts).toEqual([DEMO_HOST]);
  });

  it('always includes the standard Nikon AP address', async () => {
    const hosts = await candidateHosts();
    expect(hosts).toContain('192.168.1.1');
    for (const c of NIKON_AP_CANDIDATES) expect(hosts).toContain(c);
  });

  it('puts the preferred address first and de-dupes', async () => {
    const hosts = await candidateHosts('192.168.1.1');
    expect(hosts[0]).toBe('192.168.1.1');
    // No duplicates even though 192.168.1.1 is also in the static list.
    expect(hosts.filter((h) => h === '192.168.1.1')).toHaveLength(1);
  });

  it('prioritizes the live Wi-Fi gateway and derived guess', async () => {
    mockWifiInfo = { gateway: '10.11.12.1', ipAddress: '10.11.12.34' };
    const hosts = await candidateHosts('192.168.9.1');
    // preferred, then live gateway, then derived .1, then static candidates.
    expect(hosts[0]).toBe('192.168.9.1');
    expect(hosts[1]).toBe('10.11.12.1');
    expect(hosts).toContain('10.11.12.1');
    expect(hosts).toContain('192.168.1.1');
  });

  it('orders candidates preferred, gateway, derived-guess, then static list', async () => {
    mockWifiInfo = { gateway: '10.11.12.1', ipAddress: '192.168.5.42' };
    const hosts = await candidateHosts('172.16.0.1');
    // Exact ordering of the dynamic prefix.
    expect(hosts.slice(0, 4)).toEqual([
      '172.16.0.1', // preferred
      '10.11.12.1', // live gateway
      '192.168.5.1', // derived .1 from interface IP
      '192.168.1.1', // first static Nikon candidate
    ]);
    // Static list follows, in declared order, with no duplicates.
    for (const c of NIKON_AP_CANDIDATES) expect(hosts).toContain(c);
    expect(new Set(hosts).size).toBe(hosts.length);
  });

  it('ignores 0.0.0.0 gateway readings', async () => {
    mockWifiInfo = { gateway: '0.0.0.0', ipAddress: '0.0.0.0' };
    const hosts = await candidateHosts();
    expect(hosts).not.toContain('0.0.0.0');
    expect(hosts[0]).toBe('192.168.1.1');
  });
});
