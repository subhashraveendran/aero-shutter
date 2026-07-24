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
    setEventHandler() {
      /* noop */
    }
    setDisconnectHandler() {
      /* noop */
    }
    async close() {
      /* noop */
    }
  }
  return { PtpIpClient, defaultHostName: () => 'aero-shutter/test (test)' };
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
// Tests can flip these to drive listPhotos progress + keepAlive behavior.
type ProgressCb = (loaded: number, total: number, newPhotos: unknown[]) => void;
let cameraConnected = false;
let listPhotosImpl: (onProgress?: ProgressCb) => Promise<unknown[]> = async () => [];
const keepAliveMock = vi.fn(async () => undefined);
vi.mock('./lib/camera', () => ({
  camera: {
    get connected() {
      return cameraConnected;
    },
    cameraModel: 'Nikon D5300',
    networkBinding: 'wifi-bound',
    connect: (h: string, p?: number, b?: boolean) => cameraConnect(h, p, b),
    disconnect: async () => {
      cameraConnected = false;
    },
    listPhotos: (onProgress?: ProgressCb) => listPhotosImpl(onProgress),
    transferList: async () => [],
    keepAlive: () => keepAliveMock(),
    setObjectAddedHandler: (_cb: unknown) => undefined,
    setDisconnectHandler: (_cb: unknown) => undefined,
  },
}));

vi.mock('./lib/db', () => ({
  allImportedIds: async () => new Set<string>(),
  identityKey: (f: string, s: number) => `${f}:${s}`,
  importedToday: async () => 0,
  markImported: async () => undefined,
}));

vi.mock('./lib/settings', () => ({
  DEFAULT_SETTINGS: {
    cameraIp: '192.168.1.1',
    keepInternetOnCellular: true,
    keepAliveIntervalMs: 9000,
    autoReconnect: true,
  },
  loadSettings: async () => ({
    cameraIp: '192.168.1.1',
    keepInternetOnCellular: true,
    keepAliveIntervalMs: 9000,
    autoReconnect: true,
  }),
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

// OTA updater: let tests drive what checkForUpdate() returns so we can assert
// how checkForUpdatesManual() maps results to state + toasts.
import type { UpdateCheckResult } from './lib/updater';
const checkForUpdateMock = vi.fn(async (): Promise<UpdateCheckResult> => ({
  status: 'up-to-date',
  latest: null,
  notes: '',
  current: '0.8.3',
  native: '0.0.0-web',
}));
vi.mock('./lib/updater', () => ({
  checkForUpdate: () => checkForUpdateMock(),
  applyUpdate: vi.fn(async () => ({ pending: true })),
  reloadToApply: vi.fn(async () => undefined),
  markAppReady: vi.fn(async () => undefined),
  SHIPPED_VERSION: '0.8.3',
  WEB_NATIVE_VERSION: '0.0.0-web',
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
    photos: [],
    loadingPhotos: false,
    photoLoadProgress: null,
    keepAliveTimer: null,
    reconnecting: false,
    reconnectStatus: '',
    importing: false,
    settings: {
      cameraIp: '192.168.1.1',
      keepInternetOnCellular: true,
      keepAliveIntervalMs: 9000,
      autoReconnect: false,
    } as never,
  });

beforeEach(() => {
  probeConstructions.length = 0;
  probeShouldConnect = () => false;
  cameraConnect.mockClear();
  keepAliveMock.mockClear();
  keepAliveMock.mockResolvedValue(undefined);
  cameraConnected = false;
  listPhotosImpl = async () => [];
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

describe('refreshPhotos progressive update', () => {
  const photo = (handle: number, filename: string, epoch: number) => ({
    handle,
    filename,
    size: 100,
    format: 'JPEG',
    width: 10,
    height: 10,
    captureEpochMs: epoch,
  });

  it('sets loading + progress and appends photos as batches arrive', async () => {
    cameraConnected = true;
    const p1 = [photo(1, 'A.JPG', 300), photo(2, 'B.JPG', 200)];
    const p2 = [photo(3, 'C.JPG', 100)];
    const progressSnapshots: Array<{ loaded: number; count: number } | null> = [];

    listPhotosImpl = async (onProgress) => {
      onProgress?.(2, 3, p1);
      progressSnapshots.push({
        loaded: useStore.getState().photoLoadProgress?.loaded ?? -1,
        count: useStore.getState().photos.length,
      });
      onProgress?.(3, 3, p2);
      progressSnapshots.push({
        loaded: useStore.getState().photoLoadProgress?.loaded ?? -1,
        count: useStore.getState().photos.length,
      });
      return [...p1, ...p2];
    };

    await useStore.getState().refreshPhotos();

    // Photos filled progressively during the run.
    expect(progressSnapshots[0]).toEqual({ loaded: 2, count: 2 });
    expect(progressSnapshots[1]).toEqual({ loaded: 3, count: 3 });

    // Cleared when done.
    const s = useStore.getState();
    expect(s.loadingPhotos).toBe(false);
    expect(s.photoLoadProgress).toBeNull();
    expect(s.photos.length).toBe(3);
  });

  it('clears progress and toasts on error', async () => {
    cameraConnected = true;
    listPhotosImpl = async () => {
      throw new Error('link dropped');
    };
    useStore.setState({ toasts: [] });

    await useStore.getState().refreshPhotos();

    const s = useStore.getState();
    expect(s.loadingPhotos).toBe(false);
    expect(s.photoLoadProgress).toBeNull();
    const toast = s.toasts[s.toasts.length - 1];
    expect(toast?.kind).toBe('error');
    expect(toast?.message).toContain('link dropped');
  });
});

describe('keep-alive scheduling', () => {
  it('starts a single timer and is idempotent; stop clears it', () => {
    const setInterval = vi.spyOn(window, 'setInterval');
    const clearInterval = vi.spyOn(window, 'clearInterval');

    useStore.getState().startKeepAlive();
    const firstTimer = useStore.getState().keepAliveTimer;
    expect(firstTimer).not.toBeNull();
    // Second call is a no-op (still one timer).
    useStore.getState().startKeepAlive();
    expect(useStore.getState().keepAliveTimer).toBe(firstTimer);
    expect(setInterval).toHaveBeenCalledTimes(1);

    useStore.getState().stopKeepAlive();
    expect(useStore.getState().keepAliveTimer).toBeNull();
    expect(clearInterval).toHaveBeenCalledWith(firstTimer);

    setInterval.mockRestore();
    clearInterval.mockRestore();
  });

  it('pings the camera when connected and idle, and skips when busy', async () => {
    vi.useFakeTimers();
    cameraConnected = true;
    useStore.setState({ connected: true, importing: false, loadingPhotos: false });

    useStore.getState().startKeepAlive();
    await vi.advanceTimersByTimeAsync(9000);
    expect(keepAliveMock).toHaveBeenCalledTimes(1);

    // Busy: no ping.
    useStore.setState({ importing: true });
    await vi.advanceTimersByTimeAsync(9000);
    expect(keepAliveMock).toHaveBeenCalledTimes(1);

    useStore.getState().stopKeepAlive();
    vi.useRealTimers();
  });

  it('disconnects cleanly when keep-alive throws a link error', async () => {
    vi.useFakeTimers();
    cameraConnected = true;
    useStore.setState({ connected: true, importing: false, loadingPhotos: false, toasts: [] });
    keepAliveMock.mockRejectedValueOnce(new Error('ECONNRESET'));

    useStore.getState().startKeepAlive();
    await vi.advanceTimersByTimeAsync(9000);

    const s = useStore.getState();
    expect(s.connected).toBe(false);
    expect(s.keepAliveTimer).toBeNull();
    vi.useRealTimers();
  });
});

describe('checkForUpdatesManual', () => {
  beforeEach(() => {
    checkForUpdateMock.mockReset();
    useStore.setState({
      updateChecking: false,
      updateStatus: 'up-to-date',
      updateLatest: null,
      updateLastError: null,
      updateDismissed: false,
      toasts: [],
    });
  });

  it('reports up-to-date with the current version and a success toast', async () => {
    checkForUpdateMock.mockResolvedValue({
      status: 'up-to-date',
      latest: null,
      notes: '',
      current: '0.8.3',
      native: '0.0.0-web',
    });

    await useStore.getState().checkForUpdatesManual();

    const s = useStore.getState();
    expect(s.updateChecking).toBe(false);
    expect(s.updateCurrentVersion).toBe('0.8.3');
    expect(s.updateLastError).toBeNull();
    const toast = s.toasts[s.toasts.length - 1];
    expect(toast?.kind).toBe('success');
    expect(toast?.message).toContain('up to date');
    expect(toast?.message).toContain('0.8.3');
  });

  it('reports an available OTA update with the version toast', async () => {
    checkForUpdateMock.mockResolvedValue({
      status: 'ota-available',
      latest: { version: '0.9.0', url: 'https://example.com/b.zip', notes: '' },
      notes: '',
      current: '0.8.3',
      native: '0.8.3',
    });

    await useStore.getState().checkForUpdatesManual();

    const s = useStore.getState();
    expect(s.updateStatus).toBe('ota-available');
    expect(s.updateLatest?.version).toBe('0.9.0');
    const toast = s.toasts[s.toasts.length - 1];
    expect(toast?.message).toContain('Update available');
    expect(toast?.message).toContain('0.9.0');
  });

  it('surfaces a check error via updateLastError and an error toast', async () => {
    checkForUpdateMock.mockResolvedValue({
      status: 'up-to-date',
      latest: null,
      notes: '',
      current: '0.8.3',
      native: '0.8.3',
      error: 'offline',
    });

    await useStore.getState().checkForUpdatesManual();

    const s = useStore.getState();
    expect(s.updateLastError).toBe('offline');
    const toast = s.toasts[s.toasts.length - 1];
    expect(toast?.kind).toBe('error');
    expect(toast?.message).toContain('offline');
  });
});
