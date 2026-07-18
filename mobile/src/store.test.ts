import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The store pulls in a lot of native-adjacent modules. Mock every dependency
// that touches Capacitor / IndexedDB so the store can run under jsdom, and so
// we can observe exactly how it drives discovery + connection.

vi.mock('@aero-shutter/tcp-socket', () => ({
  isDemoMode: () => false,
}));

// Records every PtpIpClient constructed during a probe so tests can assert the
// bindWifi flag that autoConnect chose for each sweep.
const probeConstructions: Array<{ host: string; bindWifi: boolean }> = [];
let probeShouldConnect: (host: string, bindWifi: boolean) => boolean = () => false;

vi.mock('./lib/ptpip/client', () => {
  class PtpIpClient {
    constructor(
      public host: string,
      _port: number | undefined,
      _hostName: string,
      public bindWifi = true,
    ) {
      probeConstructions.push({ host, bindWifi });
    }
    async connect() {
      if (!probeShouldConnect(this.host, this.bindWifi)) {
        throw new Error('probe refused');
      }
    }
    async close() {
      /* noop */
    }
  }
  return { PtpIpClient };
});

// candidateHosts / discovery: return a fixed candidate list.
let mockHosts: string[] = ['192.168.1.1', '192.168.0.1'];
vi.mock('./lib/discovery', () => ({
  candidateHosts: vi.fn(async () => mockHosts),
}));

// wifi wrapper: let tests control join success + observe calls.
let mockJoinResult = { joined: true, ssid: 'Nikon_WU2_TEST', bound: true };
const joinWifiMock = vi.fn(async (_o: unknown) => mockJoinResult);
const findSsidMock = vi.fn(async (_p: string) => null as string | null);
const leaveWifiMock = vi.fn(async () => undefined);
vi.mock('./lib/wifi', () => ({
  NIKON_SSID_PREFIX: 'Nikon_WU2_',
  joinWifi: (o: unknown) => joinWifiMock(o),
  findSsidByPrefix: (p: string) => findSsidMock(p),
  currentWifi: async () => null,
  leaveWifi: () => leaveWifiMock(),
}));

// camera service: record connect() calls and let tests control success.
const cameraConnect = vi.fn(async (_host: string, _port?: number, _bind?: boolean) => undefined);
vi.mock('./lib/camera', () => ({
  camera: {
    get connected() {
      return false;
    },
    cameraModel: 'Nikon D5300',
    networkBinding: 'wifi-bound',
    connect: (h: string, p?: number, b?: boolean) => cameraConnect(h, p, b),
    disconnect: async () => undefined,
    listPhotos: async () => [],
  },
}));

vi.mock('./lib/db', () => ({
  allImportedIds: async () => new Set<string>(),
  identityKey: (f: string, s: number) => `${f}:${s}`,
  importedToday: async () => 0,
  markImported: async () => undefined,
}));

vi.mock('./lib/settings', () => ({
  DEFAULT_SETTINGS: { cameraIp: '192.168.1.1', keepInternetOnCellular: true },
  loadSettings: async () => ({ cameraIp: '192.168.1.1', keepInternetOnCellular: true }),
  saveSettings: async () => undefined,
}));

vi.mock('./lib/notifications', () => ({
  notifyImportComplete: async () => undefined,
  notifyNewPhoto: async () => undefined,
  requestNotificationPermission: async () => undefined,
}));

vi.mock('./lib/haptics', () => ({
  success: async () => undefined,
  warn: async () => undefined,
  tap: async () => undefined,
}));

import { useStore } from './store';

const resetStore = () =>
  useStore.setState({
    connected: false,
    connecting: false,
    connectError: null,
    connectStatus: '',
    joiningWifi: false,
    wifiError: null,
    wifiSsid: null,
    settings: { cameraIp: '192.168.1.1', keepInternetOnCellular: true } as never,
  });

beforeEach(() => {
  probeConstructions.length = 0;
  probeShouldConnect = () => false;
  cameraConnect.mockClear();
  joinWifiMock.mockClear();
  findSsidMock.mockClear();
  findSsidMock.mockResolvedValue(null);
  mockJoinResult = { joined: true, ssid: 'Nikon_WU2_TEST', bound: true };
  mockHosts = ['192.168.1.1', '192.168.0.1'];
  resetStore();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('autoConnect retry-without-bind', () => {
  it('retries the probe sweep with bindWifi=false after a bound sweep fails', async () => {
    // Bound probing never succeeds; unbound probing reaches the camera.
    probeShouldConnect = (_host, bindWifi) => bindWifi === false;

    await useStore.getState().autoConnect();

    // First sweep must have used bindWifi=true, then a second sweep bindWifi=false.
    const bindModes = probeConstructions.map((c) => c.bindWifi);
    expect(bindModes).toContain(true);
    expect(bindModes).toContain(false);
    // The bound sweep (true) must come before any unbound (false) probe.
    expect(bindModes.indexOf(true)).toBeLessThan(bindModes.indexOf(false));

    // It connected, and used the mode that actually worked (unbound).
    expect(cameraConnect).toHaveBeenCalledTimes(1);
    expect(cameraConnect).toHaveBeenCalledWith('192.168.1.1', undefined, false);
    expect(useStore.getState().connected).toBe(true);
    expect(useStore.getState().connectError).toBeNull();
  });

  it('does not run a second sweep when the bound sweep already connects', async () => {
    probeShouldConnect = (_host, bindWifi) => bindWifi === true;

    await useStore.getState().autoConnect();

    const bindModes = probeConstructions.map((c) => c.bindWifi);
    expect(bindModes).toContain(true);
    expect(bindModes).not.toContain(false);
    expect(cameraConnect).toHaveBeenCalledWith('192.168.1.1', undefined, true);
    expect(useStore.getState().connected).toBe(true);
  });

  it('surfaces an actionable error listing the tried hosts when nothing answers', async () => {
    probeShouldConnect = () => false;

    await useStore.getState().autoConnect();

    expect(useStore.getState().connected).toBe(false);
    const err = useStore.getState().connectError ?? '';
    expect(err).toContain('192.168.1.1');
    expect(err.toLowerCase()).toContain('wi-fi');
    expect(cameraConnect).not.toHaveBeenCalled();
  });

  it('always includes 192.168.1.1 even if discovery omits it', async () => {
    mockHosts = ['10.0.0.5'];
    probeShouldConnect = (host) => host === '192.168.1.1';

    await useStore.getState().autoConnect();

    const probedHosts = probeConstructions.map((c) => c.host);
    expect(probedHosts).toContain('192.168.1.1');
    expect(useStore.getState().connected).toBe(true);
  });
});

describe('joinCameraWifi → autoConnect', () => {
  it('joins the camera Wi-Fi then auto-connects on success', async () => {
    // Default prefix path: no exact SSID, scan finds nothing, prefix join wins.
    probeShouldConnect = () => true; // any probe reaches the camera

    await useStore.getState().joinCameraWifi();

    // Joined via the Nikon prefix, updated the displayed SSID.
    expect(joinWifiMock).toHaveBeenCalledTimes(1);
    const arg = joinWifiMock.mock.calls[0][0] as { ssidPrefix?: string };
    expect(arg.ssidPrefix).toBe('Nikon_WU2_');
    expect(useStore.getState().wifiSsid).toBe('Nikon_WU2_TEST');

    // Auto-connect ran and succeeded.
    expect(cameraConnect).toHaveBeenCalledTimes(1);
    expect(useStore.getState().connected).toBe(true);
    expect(useStore.getState().joiningWifi).toBe(false);
    expect(useStore.getState().wifiError).toBeNull();
  });

  it('passes an exact SSID + password straight through and does not scan', async () => {
    probeShouldConnect = () => true;

    await useStore.getState().joinCameraWifi('Nikon_WU2_ABCDEF', 'hunter2');

    const arg = joinWifiMock.mock.calls[0][0] as {
      ssid?: string;
      password?: string;
      ssidPrefix?: string;
    };
    expect(arg.ssid).toBe('Nikon_WU2_ABCDEF');
    expect(arg.password).toBe('hunter2');
    expect(arg.ssidPrefix).toBeUndefined();
    expect(findSsidMock).not.toHaveBeenCalled();
    expect(useStore.getState().connected).toBe(true);
  });

  it('surfaces an error and does not connect when the join fails', async () => {
    mockJoinResult = { joined: false, ssid: 'Nikon_WU2_', bound: false };

    await useStore.getState().joinCameraWifi();

    expect(cameraConnect).not.toHaveBeenCalled();
    expect(useStore.getState().connected).toBe(false);
    expect(useStore.getState().joiningWifi).toBe(false);
    expect((useStore.getState().wifiError ?? '').toLowerCase()).toContain('wi-fi');
  });

  it('is re-entrant safe: ignores a second call while already joining', async () => {
    useStore.setState({ joiningWifi: true });
    await useStore.getState().joinCameraWifi();
    expect(joinWifiMock).not.toHaveBeenCalled();
  });
});
