import { describe, expect, it } from 'vitest';
import { checkForUpdate, decideUpdate, type OtaManifest } from './updater';

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
  });
});
