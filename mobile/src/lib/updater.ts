// Self-hosted, $0 over-the-air (OTA) live updates.
//
// We use the OPEN-SOURCE @capgo/capacitor-updater plugin via its low-level
// programmatic API only — there is NO Capgo Cloud account and no paid service.
// The whole flow is driven from here against a manifest we host on our own
// GitHub Releases:
//
//   1. On startup: CapacitorUpdater.notifyAppReady() (rollback safety net).
//   2. checkForUpdate(): fetch ota.json (cache-busted), compare its `version`
//      to the currently-running bundle version, and compare its
//      `minNativeVersion` to the installed native (APK) version.
//        - newer web bundle + native OK  -> 'ota-available'
//        - newer bundle needs newer APK   -> 'native-required'
//        - otherwise                      -> 'up-to-date'
//   3. applyUpdate(): download() the zip, then next() it — queued to apply on
//      the next reload (does NOT interrupt the user; connect flow keeps going).
//   4. reload() to swap the webview to the queued bundle when the user chooses.
//
// Everything NO-OPs gracefully on web / demo (no native plugin present): checks
// resolve to 'up-to-date' and apply/reload do nothing.

import { Capacitor, CapacitorHttp } from '@capacitor/core';
import { compareVersions, isNewer, satisfiesMin } from './semver';

/** Stable "latest" download URLs on our own GitHub Releases (no Capgo cloud). */
const OTA_MANIFEST_URL =
  'https://github.com/subhashraveendran/aero-shutter/releases/latest/download/ota.json';

/** Where users are sent when a full APK (native) update is required. */
export const RELEASES_PAGE_URL =
  'https://github.com/subhashraveendran/aero-shutter/releases/latest';

/** Shipped web bundle version, injected from mobile/package.json at build. */
const SHIPPED_VERSION: string =
  typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0';

/** Version reported on web / demo where there is no native shell. */
const WEB_NATIVE_VERSION = '0.0.0-web';

/** Parsed shape of ota.json hosted on GitHub Releases. */
export interface OtaManifest {
  /** Semver of the web bundle available for download, e.g. "0.8.0". */
  version: string;
  /** Absolute URL of the web-bundle.zip to download. */
  url: string;
  /** Human-readable release notes. */
  notes?: string;
  /** Minimum native (APK) version required to run this bundle. */
  minNativeVersion?: string;
}

export type UpdateStatus = 'up-to-date' | 'ota-available' | 'native-required';

export interface UpdateCheckResult {
  status: UpdateStatus;
  /** The manifest we fetched, when a network check actually ran. */
  latest: OtaManifest | null;
  /** Convenience copy of the notes for the UI. */
  notes: string;
  /** Currently-running web bundle version (for display). */
  current: string;
  /** Installed native (APK) version (for display). */
  native: string;
  /** Populated when the check failed (offline / blocked / malformed). */
  error?: string;
}

/** True when running inside a native Capacitor shell (not web / demo). */
export function isNative(): boolean {
  try {
    return Capacitor.isNativePlatform();
  } catch {
    return false;
  }
}

// The plugin/App modules are imported lazily so the web/demo bundle never has
// to resolve a native-only plugin at module load. Typed loosely to avoid a
// hard compile-time coupling to the plugin's evolving surface.
type UpdaterPlugin = {
  notifyAppReady: () => Promise<unknown>;
  current: () => Promise<{ bundle: { version: string }; native: string }>;
  download: (opts: { url: string; version: string }) => Promise<{ id: string; version: string }>;
  // next() queues the bundle to activate on the next reload WITHOUT reloading
  // now — so applying an update never interrupts the user mid-flow.
  next: (opts: { id: string }) => Promise<{ id: string; version: string }>;
  reload: () => Promise<void>;
  addListener: (
    event: 'download',
    cb: (info: { percent: number }) => void,
  ) => Promise<{ remove: () => Promise<void> }>;
};

async function getUpdater(): Promise<UpdaterPlugin | null> {
  if (!isNative()) return null;
  try {
    const mod = await import('@capgo/capacitor-updater');
    const p = mod.CapacitorUpdater as unknown as UpdaterPlugin;
    // IMPORTANT: do NOT return the raw plugin object. Capacitor plugins are
    // Proxies that forward EVERY property access to native — including `.then`.
    // Returning that thenable from an async function makes the runtime try to
    // adopt it (call `.then()`), which the bridge rejects with
    // "CapacitorUpdater.then() is not implemented on android". That rejection
    // breaks markAppReady() and, critically, applyUpdate() — so OTA silently
    // never applies. Wrap the methods in a plain (non-thenable) object instead.
    return {
      notifyAppReady: () => p.notifyAppReady(),
      current: () => p.current(),
      download: (opts) => p.download(opts),
      next: (opts) => p.next(opts),
      reload: () => p.reload(),
      addListener: (event, cb) => p.addListener(event, cb),
    };
  } catch {
    return null;
  }
}

/** Installed native (APK) version, via @capacitor/app. Web/demo => constant. */
export async function getNativeVersion(): Promise<string> {
  if (!isNative()) return WEB_NATIVE_VERSION;
  try {
    const { App } = await import('@capacitor/app');
    const info = await App.getInfo();
    return info.version || SHIPPED_VERSION;
  } catch {
    return SHIPPED_VERSION;
  }
}

/**
 * Version of the currently-running web bundle. Prefers the Capgo plugin's
 * record of the active bundle; falls back to the shipped (built-in) version
 * when the plugin has no downloaded bundle yet. Web/demo => null.
 */
export async function getCurrentBundleVersion(): Promise<string | null> {
  if (!isNative()) return null;
  const updater = await getUpdater();
  if (!updater) return SHIPPED_VERSION;
  try {
    const cur = await updater.current();
    const v = cur?.bundle?.version;
    // "builtin" bundles report the native version; treat that as the shipped
    // version so we don't misread it as an ancient bundle.
    if (!v || v === 'builtin' || v === 'unknown') return SHIPPED_VERSION;
    return v;
  } catch {
    return SHIPPED_VERSION;
  }
}

/** Call once on startup so a bad freshly-set bundle rolls back automatically. */
export async function markAppReady(): Promise<void> {
  const updater = await getUpdater();
  if (!updater) return;
  try {
    await updater.notifyAppReady();
  } catch {
    /* non-fatal */
  }
}

/** Fetch and parse ota.json with a cache-busting query param. */
async function fetchManifest(signal?: AbortSignal): Promise<OtaManifest | null> {
  const url = `${OTA_MANIFEST_URL}?t=${Date.now()}`;
  let data: Partial<OtaManifest>;
  if (isNative()) {
    // Use Capacitor's native HTTP, NOT window.fetch: inside the app webview a
    // request to github.com is cross-origin, and GitHub's asset host does not
    // send Access-Control-Allow-Origin, so a plain fetch() is blocked by CORS
    // and the update check silently fails. Native HTTP has no CORS and follows
    // the release "latest/download" redirect.
    const res = await CapacitorHttp.get({ url, headers: { Accept: 'application/json' } });
    if (res.status < 200 || res.status >= 300) throw new Error(`manifest HTTP ${res.status}`);
    data = (typeof res.data === 'string' ? JSON.parse(res.data) : res.data) as Partial<OtaManifest>;
  } else {
    const res = await fetch(url, { cache: 'no-store', signal });
    if (!res.ok) throw new Error(`manifest HTTP ${res.status}`);
    data = (await res.json()) as Partial<OtaManifest>;
  }
  if (!data || typeof data.version !== 'string' || typeof data.url !== 'string') {
    throw new Error('malformed ota.json');
  }
  return {
    version: data.version,
    url: data.url,
    notes: typeof data.notes === 'string' ? data.notes : '',
    minNativeVersion:
      typeof data.minNativeVersion === 'string' ? data.minNativeVersion : undefined,
  };
}

/**
 * Decide what to do given a manifest and the installed versions. Pure, so it's
 * easy to unit-test all three outcomes.
 */
export function decideUpdate(
  manifest: OtaManifest,
  currentBundleVersion: string,
  nativeVersion: string,
): UpdateStatus {
  const notes = manifest.notes ?? '';
  void notes;
  // Nothing newer on offer.
  if (!isNewer(manifest.version, currentBundleVersion)) return 'up-to-date';
  // A newer bundle exists. Can the installed APK run it?
  if (manifest.minNativeVersion && !satisfiesMin(nativeVersion, manifest.minNativeVersion)) {
    return 'native-required';
  }
  return 'ota-available';
}

/**
 * Check our GitHub-hosted manifest for a newer web bundle. NO-OPs to
 * 'up-to-date' on web / demo or on any error (never blocks the app).
 */
export async function checkForUpdate(signal?: AbortSignal): Promise<UpdateCheckResult> {
  if (!isNative()) {
    return {
      status: 'up-to-date',
      latest: null,
      notes: '',
      current: SHIPPED_VERSION,
      native: WEB_NATIVE_VERSION,
    };
  }
  const [bundleVersion, nativeVersion] = await Promise.all([
    getCurrentBundleVersion(),
    getNativeVersion(),
  ]);
  const current = bundleVersion ?? SHIPPED_VERSION;
  try {
    const manifest = await fetchManifest(signal);
    if (!manifest) {
      return { status: 'up-to-date', latest: null, notes: '', current, native: nativeVersion };
    }
    const status = decideUpdate(manifest, current, nativeVersion);
    return { status, latest: manifest, notes: manifest.notes ?? '', current, native: nativeVersion };
  } catch (e) {
    // Offline / rate-limited / malformed manifest. Surface the error for the
    // manual "Check for updates" UI; the auto-check treats it as up-to-date.
    return {
      status: 'up-to-date',
      latest: null,
      notes: '',
      current,
      native: nativeVersion,
      error: e instanceof Error ? e.message : 'update check failed',
    };
  }
}

export interface ApplyResult {
  /** True once the bundle is downloaded + set and pending a reload. */
  pending: boolean;
  error?: string;
}

/**
 * Download the new bundle and set it as the next bundle to load. Reports
 * download progress (0-100) through the optional callback. Does NOT reload —
 * call reloadToApply() when the user chooses to restart. NO-OPs on web/demo.
 */
export async function applyUpdate(
  latest: OtaManifest,
  onProgress?: (percent: number) => void,
): Promise<ApplyResult> {
  const updater = await getUpdater();
  if (!updater) return { pending: false, error: 'OTA not available on this platform' };

  let remove: (() => Promise<void>) | null = null;
  try {
    if (onProgress) {
      try {
        const handle = await updater.addListener('download', (info) => {
          if (typeof info?.percent === 'number') onProgress(info.percent);
        });
        remove = handle.remove;
      } catch {
        /* progress events optional */
      }
    }
    const bundle = await updater.download({ url: latest.url, version: latest.version });
    // Queue (not set) so nothing reloads until the user taps "Restart to apply".
    await updater.next({ id: bundle.id });
    return { pending: true };
  } catch (e) {
    return { pending: false, error: e instanceof Error ? e.message : 'Update failed' };
  } finally {
    if (remove) await remove().catch(() => undefined);
  }
}

/** Reload the webview to swap in the pending bundle. NO-OPs on web/demo. */
export async function reloadToApply(): Promise<void> {
  const updater = await getUpdater();
  if (!updater) {
    // Web fallback: a plain reload (harmless in demo).
    if (typeof window !== 'undefined') window.location.reload();
    return;
  }
  try {
    await updater.reload();
  } catch {
    if (typeof window !== 'undefined') window.location.reload();
  }
}

export { SHIPPED_VERSION, WEB_NATIVE_VERSION, compareVersions };
