import { afterEach, describe, expect, it, vi } from 'vitest';

// Mock Capacitor so we can flip native mode and force the CapacitorHttp path to
// fail without any real network. Defaults to web (isNativePlatform false).
let mockIsNative = false;
const capacitorHttpGet = vi.fn();
vi.mock('@capacitor/core', () => ({
  Capacitor: { isNativePlatform: () => mockIsNative },
  CapacitorHttp: { get: (o: unknown) => capacitorHttpGet(o) },
}));

// Native-only plugins the updater lazy-imports; give them deterministic stubs
// so the native code path doesn't touch the real (web-unimplemented) plugins.
vi.mock('@capgo/capacitor-updater', () => ({
  CapacitorUpdater: {
    notifyAppReady: async () => undefined,
    current: async () => ({ bundle: { version: '0.8.3' }, native: '0.8.3' }),
  },
}));
vi.mock('@capacitor/app', () => ({
  App: { getInfo: async () => ({ version: '0.8.3' }) },
}));

import {
  checkForUpdate,
  decideUpdate,
  SHIPPED_VERSION,
  WEB_NATIVE_VERSION,
  type OtaManifest,
} from './updater';

const manifest = (over: Partial<OtaManifest> = {}): OtaManifest => ({
  version: '0.8.0',
  url: 'https://example.com/web-bundle.zip',
  notes: 'notes',
  minNativeVersion: '0.7.0',
  ...over,
});

describe('decideUpdate', () => {
  it('returns up-to-date when the manifest is not newer than the running bundle', () => {
    // same version
    expect(decideUpdate(manifest({ version: '0.7.0' }), '0.7.0', '0.7.0')).toBe('up-to-date');
    // older manifest than what is running
    expect(decideUpdate(manifest({ version: '0.6.0' }), '0.7.0', '0.7.0')).toBe('up-to-date');
  });

  it('returns ota-available when a newer bundle runs on the installed native version', () => {
    expect(decideUpdate(manifest({ version: '0.8.0', minNativeVersion: '0.7.0' }), '0.7.0', '0.7.0')).toBe(
      'ota-available',
    );
    // no minNativeVersion => always OTA-eligible
    expect(
      decideUpdate(manifest({ version: '0.9.0', minNativeVersion: undefined }), '0.7.0', '0.7.0'),
    ).toBe('ota-available');
  });

  it('returns native-required when the newer bundle needs a newer APK than installed', () => {
    expect(
      decideUpdate(manifest({ version: '0.9.0', minNativeVersion: '0.9.0' }), '0.7.0', '0.7.0'),
    ).toBe('native-required');
  });
});

describe('checkForUpdate on web/demo', () => {
  it('no-ops to up-to-date (no native platform, no network needed)', async () => {
    const result = await checkForUpdate();
    expect(result.status).toBe('up-to-date');
    expect(result.latest).toBeNull();
    // Reports the shipped/web constants so the UI has something to display.
    expect(result.current).toBe(SHIPPED_VERSION);
    expect(result.native).toBe(WEB_NATIVE_VERSION);
    expect(result.error).toBeUndefined();
  });
});

describe('checkForUpdate on native with a failing manifest fetch', () => {
  afterEach(() => {
    mockIsNative = false;
    capacitorHttpGet.mockReset();
  });

  it('populates error while status stays up-to-date (never blocks the app)', async () => {
    // Pretend we're inside the native shell so the CapacitorHttp path runs.
    mockIsNative = true;
    // Fail the manifest fetch (offline / blocked) — no real network.
    capacitorHttpGet.mockRejectedValue(new Error('offline'));

    const result = await checkForUpdate();

    expect(result.status).toBe('up-to-date');
    expect(result.latest).toBeNull();
    expect(result.error).toBe('offline');
    // Still reports a current version for display.
    expect(typeof result.current).toBe('string');
  });
});
