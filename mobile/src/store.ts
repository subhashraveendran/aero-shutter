import { create } from 'zustand';
import { isDemoMode } from '@aero-shutter/tcp-socket';
import { camera, type CameraProperty, type Photo } from './lib/camera';
import {
  allImportedIds,
  identityKey,
  importedToday as countImportedToday,
  markImported,
} from './lib/db';
import {
  DEFAULT_SETTINGS,
  loadSettings,
  saveSettings,
  type Settings,
} from './lib/settings';
import { candidateHosts } from './lib/discovery';
import {
  currentWifi as readCurrentWifi,
  findSsidByPrefix,
  joinWifi as joinWifiNative,
  leaveWifi as leaveWifiNative,
  NIKON_SSID_PREFIX,
} from './lib/wifi';
import { PtpIpClient } from './lib/ptpip/client';
import {
  notifyImportComplete,
  notifyNewPhoto,
  requestNotificationPermission,
} from './lib/notifications';
import * as haptics from './lib/haptics';
import {
  applyUpdate,
  checkForUpdate,
  markAppReady,
  reloadToApply,
  SHIPPED_VERSION,
  WEB_NATIVE_VERSION,
  type OtaManifest,
  type UpdateStatus,
} from './lib/updater';

export type Screen = 'connect' | 'gallery' | 'detail' | 'import' | 'control' | 'settings';
export type FilterChip = 'all' | 'new' | 'raw' | 'jpeg' | 'imported';

export interface ImportTask {
  photo: Photo;
  status: 'pending' | 'active' | 'done' | 'error';
  bytesDone: number;
  totalBytes: number;
  error?: string;
  startedAt?: number;
}

export interface Toast {
  id: number;
  message: string;
  kind: 'info' | 'success' | 'error';
}

interface AppState {
  demo: boolean;
  screen: Screen;
  connected: boolean;
  connecting: boolean;
  connectError: string | null;
  cameraModel: string;
  /** Human-readable progress during auto-detect, e.g. "Found camera at 192.168.1.1". */
  connectStatus: string;
  /** How the camera socket was bound: 'wifi-bound' | 'wifi-pinned' | 'default' | 'unsupported'. */
  networkBinding: string | null;

  /** SSID the app is currently joined to (best-effort), for display. */
  wifiSsid: string | null;
  /** True while an in-app Wi-Fi join is in flight. */
  joiningWifi: boolean;
  /** Last in-app Wi-Fi join error, if any. */
  wifiError: string | null;

  settings: Settings;

  photos: Photo[];
  loadingPhotos: boolean;
  /** Progress while listing photos, or null when idle/done. */
  photoLoadProgress: { loaded: number; total: number } | null;
  importedIds: Set<string>;
  filter: FilterChip;
  selection: Set<number>;

  detailIndex: number;

  props: CameraProperty[];
  loadingProps: boolean;

  importQueue: ImportTask[];
  importing: boolean;
  importedTodayCount: number;

  toasts: Toast[];

  autoImportTimer: number | null;
  /** Keep-alive interval handle; pings the camera so it doesn't idle-drop. */
  keepAliveTimer: number | null;

  // OTA live-update state (self-hosted; no-ops on web/demo).
  /** Result of the last manifest check. 'up-to-date' until proven otherwise. */
  updateStatus: UpdateStatus;
  /** The manifest for an available OTA update, if any. */
  updateLatest: OtaManifest | null;
  /** True while downloading/applying the bundle. */
  updateApplying: boolean;
  /** Download progress 0-100 while applying. */
  updateProgress: number;
  /** True once a bundle is downloaded + set, pending a restart. */
  updatePending: boolean;
  /** User dismissed the current update banner. */
  updateDismissed: boolean;
  /** Currently-running web bundle version (for the Settings display). */
  updateCurrentVersion: string;
  /** Installed native (APK) version, or the web sentinel on web/demo. */
  updateNativeVersion: string;
  /** Last update-check error, if any (shown in the Settings section). */
  updateLastError: string | null;
  /** True while a manual "Check for updates" is in flight. */
  updateChecking: boolean;

  // actions
  init: () => Promise<void>;
  navigate: (screen: Screen) => void;
  connect: (ip?: string) => Promise<void>;
  autoConnect: () => Promise<void>;
  /**
   * Join the camera's Wi-Fi from inside the app, then kick off autoConnect().
   * Pass an exact SSID, or omit to use the Nikon prefix ("Nikon_WU2_").
   */
  joinCameraWifi: (ssidOrPrefix?: string, password?: string) => Promise<void>;
  /** Refresh the displayed current-Wi-Fi SSID (best-effort). */
  refreshWifiSsid: () => Promise<void>;
  /** Internal: finalize state after a successful handshake + session open. */
  _onConnected: (host: string) => Promise<void>;
  enterDemo: () => Promise<void>;
  disconnect: () => Promise<void>;
  refreshPhotos: () => Promise<void>;
  setFilter: (f: FilterChip) => void;
  toggleSelect: (handle: number) => void;
  clearSelection: () => void;
  openDetail: (index: number) => void;
  setDetailIndex: (index: number) => void;
  importPhotos: (photos: Photo[]) => Promise<void>;
  importNew: () => Promise<void>;
  cancelImport: () => void;
  loadProps: () => Promise<void>;
  changeProp: (prop: CameraProperty, value: number) => Promise<void>;
  capture: () => Promise<void>;
  updateSettings: (patch: Partial<Settings>) => Promise<void>;
  toast: (message: string, kind?: Toast['kind']) => void;
  dismissToast: (id: number) => void;
  startAutoImport: () => void;
  stopAutoImport: () => void;
  /** Start the periodic keep-alive ping (no-op if already running). */
  startKeepAlive: () => void;
  /** Stop the keep-alive ping. */
  stopKeepAlive: () => void;

  /** Check the OTA manifest for a newer web bundle (debounced, demo-safe). */
  checkForUpdates: () => Promise<void>;
  /**
   * User-triggered update check: always runs (no debounce), records versions +
   * error, and shows a toast summarizing the outcome. Demo-safe.
   */
  checkForUpdatesManual: () => Promise<void>;
  /** Download + set the pending OTA bundle, updating progress state. */
  runUpdate: () => Promise<void>;
  /** Reload the webview to apply the pending bundle. */
  applyPendingUpdate: () => Promise<void>;
  /** Dismiss the update banner for this session. */
  dismissUpdate: () => void;
}

let toastSeq = 1;
let importCancelled = false;

// OTA manifest check debounce: ignore repeated checks within this window
// (launch + every resume can both fire checkForUpdates()).
const UPDATE_CHECK_COOLDOWN_MS = 60_000;
let lastUpdateCheck = 0;

function filteredNewCount(photos: Photo[], imported: Set<string>): number {
  return photos.filter((p) => !imported.has(identityKey(p.filename, p.size))).length;
}

/**
 * Lightweight reachability probe: open a throwaway PTP/IP connection to `host`,
 * confirm the handshake, then close it. Resolves if the camera answered,
 * rejects on timeout / refusal. Used to race candidate addresses.
 */
async function probeHost(host: string, timeoutMs: number, bindWifi: boolean): Promise<void> {
  const client = new PtpIpClient(host, undefined, 'AeroShutter', bindWifi);
  try {
    await client.connect(timeoutMs);
  } finally {
    // Always release the probe socket; the winner reconnects fresh.
    await client.close().catch(() => undefined);
  }
}

export const useStore = create<AppState>((set, get) => ({
  demo: isDemoMode(),
  screen: 'connect',
  connected: false,
  connecting: false,
  connectError: null,
  cameraModel: '',
  connectStatus: '',
  networkBinding: null,

  wifiSsid: null,
  joiningWifi: false,
  wifiError: null,

  settings: { ...DEFAULT_SETTINGS },

  photos: [],
  loadingPhotos: false,
  photoLoadProgress: null,
  importedIds: new Set(),
  filter: 'all',
  selection: new Set(),

  detailIndex: 0,

  props: [],
  loadingProps: false,

  importQueue: [],
  importing: false,
  importedTodayCount: 0,

  toasts: [],
  autoImportTimer: null,
  keepAliveTimer: null,

  updateStatus: 'up-to-date',
  updateLatest: null,
  updateApplying: false,
  updateProgress: 0,
  updatePending: false,
  updateDismissed: false,
  updateCurrentVersion: SHIPPED_VERSION,
  updateNativeVersion: WEB_NATIVE_VERSION,
  updateLastError: null,
  updateChecking: false,

  async init() {
    const settings = await loadSettings();
    const importedIds = await allImportedIds();
    const importedTodayCount = await countImportedToday();
    document.documentElement.dataset.theme = settings.theme;
    set({ settings, importedIds, importedTodayCount });
    // OTA: tell the native layer we booted OK (rollback safety net), then
    // check our self-hosted manifest for a newer web bundle. Both no-op on web.
    void markAppReady();
    void get().checkForUpdates();
  },

  navigate(screen) {
    set({ screen });
  },

  async connect(ip) {
    // With no explicit ip, auto-detect the camera across candidate addresses.
    if (ip === undefined) {
      await get().autoConnect();
      return;
    }
    if (get().connecting || get().connected) return;
    const bindWifi = get().settings.keepInternetOnCellular;
    set({ connecting: true, connectError: null, connectStatus: `Connecting to ${ip}…` });
    try {
      await camera.connect(ip, undefined, bindWifi);
      if (ip !== get().settings.cameraIp && !get().demo) {
        void get().updateSettings({ cameraIp: ip });
      }
      await get()._onConnected(ip);
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Could not reach the camera';
      set({
        connecting: false,
        connectError:
          `Could not reach ${ip}: ${message}. Check the address and that your phone is on ` +
          `the camera's Wi-Fi.`,
        connectStatus: '',
      });
      await haptics.warn();
    }
  },

  async autoConnect() {
    // Safe to call repeatedly: bail if we're already connecting or connected.
    if (get().connecting || get().connected) return;
    set({ connecting: true, connectError: null, connectStatus: 'Searching for camera…' });
    const preferBind = get().settings.keepInternetOnCellular;

    let hosts: string[];
    try {
      hosts = await candidateHosts(get().settings.cameraIp);
    } catch {
      hosts = [get().settings.cameraIp || '192.168.1.1'];
    }
    // Guarantee the standard Nikon AP address is always in play, even if
    // discovery returned an empty or odd list.
    if (!hosts.includes('192.168.1.1')) hosts = [...hosts, '192.168.1.1'];

    const PROBE_TIMEOUT = get().demo ? 8000 : 2500;

    // One probe sweep: race all candidate hosts with the given bind mode; the
    // first to finish the PTP/IP handshake wins. Returns null if none answer.
    const probeSweep = (bindWifi: boolean): Promise<string | null> =>
      new Promise<string | null>((resolve) => {
        let settled = false;
        let remaining = hosts.length;
        if (remaining === 0) {
          resolve(null);
          return;
        }
        for (const host of hosts) {
          void (async () => {
            try {
              await probeHost(host, PROBE_TIMEOUT, bindWifi);
              if (!settled) {
                settled = true;
                set({ connectStatus: `Found camera at ${host}` });
                resolve(host);
              }
            } catch {
              remaining -= 1;
              if (remaining === 0 && !settled) {
                settled = true;
                resolve(null);
              }
            }
          })();
        }
      });

    // First sweep with the user's preferred bind mode. If bindWifi probing
    // fails for every candidate, RETRY the whole sweep once WITHOUT Wi-Fi
    // binding before declaring failure — this covers the no-cellular case
    // (default network already IS the camera Wi-Fi) and binding quirks.
    let winner = await probeSweep(preferBind);
    let bindWifi = preferBind;
    if (!winner && preferBind) {
      set({ connectStatus: 'Retrying without Wi-Fi binding…' });
      winner = await probeSweep(false);
      if (winner) bindWifi = false;
    }

    if (!winner) {
      const tried = hosts.join(', ');
      set({
        connecting: false,
        connectStatus: '',
        connectError:
          `No camera found (tried ${tried}). Make sure your phone is joined to the ` +
          `camera's Wi-Fi (e.g. “Nikon_WU2_…”), or enter the IP manually below.`,
      });
      await haptics.warn();
      return;
    }

    // Establish the real, session-open connection on the winning host, using
    // whichever bind mode actually reached the camera during probing.
    try {
      await camera.connect(winner, undefined, bindWifi);
      // Remember the working address for next time.
      if (winner !== get().settings.cameraIp && !get().demo) {
        void get().updateSettings({ cameraIp: winner });
      }
      await get()._onConnected(winner);
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Could not reach the camera';
      set({
        connecting: false,
        connectError: `Could not open a session with ${winner}: ${message}`,
        connectStatus: '',
      });
      await haptics.warn();
    }
  },

  async joinCameraWifi(ssidOrPrefix, password) {
    // Re-entrancy guard: ignore if a join or connect is already running.
    if (get().joiningWifi || get().connecting || get().connected) return;

    // Decide whether the caller gave a full SSID or a prefix to auto-find.
    const input = (ssidOrPrefix ?? '').trim();
    const prefix = input && input.endsWith('_') ? input : input ? undefined : NIKON_SSID_PREFIX;
    const exactSsid = input && !input.endsWith('_') ? input : undefined;

    set({ joiningWifi: true, wifiError: null, connectStatus: 'Joining camera Wi-Fi…' });
    try {
      // If we only have a prefix, try a scan first to resolve a concrete SSID
      // (nicer display, and lets the native exact-join path run). Falls through
      // to a prefix join if scanning is denied / empty.
      let ssid = exactSsid;
      const usePrefix = prefix ?? (exactSsid ? undefined : NIKON_SSID_PREFIX);
      if (!ssid && usePrefix) {
        const found = await findSsidByPrefix(usePrefix);
        if (found) ssid = found;
      }

      const result = await joinWifiNative({
        ssid,
        password: password || undefined,
        ssidPrefix: ssid ? undefined : usePrefix,
      });

      if (!result.joined) {
        set({
          joiningWifi: false,
          connectStatus: '',
          wifiError:
            'Could not join the camera Wi-Fi automatically. Open your phone’s Wi-Fi ' +
            'settings and connect to “' + (result.ssid || NIKON_SSID_PREFIX) + '…”, then retry.',
        });
        await haptics.warn();
        return;
      }

      set({ wifiSsid: result.ssid || ssid || null, joiningWifi: false });
      await haptics.success();
      // The camera AP is now the app's network — auto-connect straight away.
      await get().autoConnect();
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Wi-Fi join failed';
      set({ joiningWifi: false, connectStatus: '', wifiError: message });
      await haptics.warn();
    }
  },

  async refreshWifiSsid() {
    try {
      const ssid = await readCurrentWifi();
      if (ssid !== get().wifiSsid) set({ wifiSsid: ssid });
    } catch {
      /* best effort */
    }
  },

  async _onConnected(host: string) {
    set({
      connected: true,
      connecting: false,
      cameraModel: camera.cameraModel,
      networkBinding: camera.networkBinding,
      connectStatus: `Connected to ${camera.cameraModel || host}`,
      screen: 'gallery',
    });
    await haptics.success();
    // Ask for notification permission up-front so later import events can fire.
    if (get().settings.notifyOnImport) void requestNotificationPermission();
    // Keep the PTP session + Wi-Fi link alive against the camera's idle timeout.
    get().startKeepAlive();
    await get().refreshPhotos();
    if (get().settings.autoImport && get().settings.watchMode) get().startAutoImport();
  },

  async enterDemo() {
    // Demo mode uses the same code path; the host is irrelevant to the mock.
    await get().connect('127.0.0.1');
  },

  async disconnect() {
    const t = get().autoImportTimer;
    if (t) window.clearInterval(t);
    get().stopKeepAlive();
    await camera.disconnect();
    // Release any app-requested Wi-Fi binding so the phone can return to its
    // normal network. Best-effort; no-op if we never joined in-app.
    await leaveWifiNative();
    set({
      connected: false,
      photos: [],
      selection: new Set(),
      props: [],
      screen: 'connect',
      autoImportTimer: null,
      keepAliveTimer: null,
      photoLoadProgress: null,
      connectStatus: '',
      networkBinding: null,
      wifiSsid: null,
    });
  },

  async refreshPhotos() {
    if (!camera.connected) return;
    set({ loadingPhotos: true, photoLoadProgress: { loaded: 0, total: 0 } });
    try {
      // Progressive: listPhotos yields newest-first batches. We accumulate the
      // decoded photos and set the growing array so the contact sheet fills as
      // items arrive instead of blocking on a spinner for 500+ objects.
      const accumulated: Photo[] = [];
      const photos = await camera.listPhotos((loaded, total, newPhotos) => {
        // Ignore stale callbacks if we disconnected mid-listing.
        if (!camera.connected) return;
        accumulated.push(...newPhotos);
        set({
          photos: [...accumulated],
          photoLoadProgress: { loaded, total },
        });
      });
      set({ photos, loadingPhotos: false, photoLoadProgress: null });
    } catch (e) {
      set({ loadingPhotos: false, photoLoadProgress: null });
      get().toast(e instanceof Error ? e.message : 'Failed to load photos', 'error');
    }
  },

  setFilter(filter) {
    set({ filter });
  },

  toggleSelect(handle) {
    const selection = new Set(get().selection);
    if (selection.has(handle)) selection.delete(handle);
    else selection.add(handle);
    set({ selection });
    void haptics.tap();
  },

  clearSelection() {
    set({ selection: new Set() });
  },

  openDetail(index) {
    set({ detailIndex: index, screen: 'detail' });
  },

  setDetailIndex(index) {
    set({ detailIndex: index });
  },

  async importPhotos(photos) {
    if (get().settings.destination === 'off') {
      get().toast('Destination is set to Off. Choose one in Settings.', 'error');
      return;
    }
    const queue: ImportTask[] = photos.map((photo) => ({
      photo,
      status: 'pending',
      bytesDone: 0,
      totalBytes: photo.size,
    }));
    importCancelled = false;
    set({ importQueue: queue, importing: true, screen: 'import' });

    const destination = get().settings.destination;
    for (let i = 0; i < queue.length; i++) {
      if (importCancelled) break;
      const task = queue[i];
      task.status = 'active';
      task.startedAt = Date.now();
      set({ importQueue: [...queue] });
      try {
        const { path } = await camera.importPhoto(task.photo, destination, (done, total) => {
          task.bytesDone = done;
          task.totalBytes = total;
          set({ importQueue: [...queue] });
        });
        await markImported({
          handle: task.photo.handle,
          filename: task.photo.filename,
          size: task.photo.size,
          format: task.photo.format,
          importedAt: Date.now(),
          destination,
          path,
        });
        task.status = 'done';
      } catch (e) {
        task.status = 'error';
        task.error = e instanceof Error ? e.message : 'Import failed';
      }
      set({ importQueue: [...queue] });
    }

    const importedIds = await allImportedIds();
    const importedTodayCount = await countImportedToday();
    set({ importing: false, importedIds, importedTodayCount, selection: new Set() });
    if (importCancelled) {
      const doneCount = queue.filter((t) => t.status === 'done').length;
      await haptics.warn();
      get().toast(`Cancelled — ${doneCount} imported`, 'info');
      importCancelled = false;
      return;
    }
    const errors = queue.filter((t) => t.status === 'error').length;
    const doneCount = queue.filter((t) => t.status === 'done').length;
    if (get().settings.notifyOnImport && doneCount > 0) {
      void notifyImportComplete(doneCount);
    }
    if (errors === 0) {
      await haptics.success();
      get().toast(`Developed ${queue.length} frame${queue.length === 1 ? '' : 's'}`, 'success');
    } else {
      await haptics.warn();
      get().toast(`${errors} import${errors === 1 ? '' : 's'} failed`, 'error');
    }
  },

  cancelImport() {
    importCancelled = true;
  },

  async importNew() {
    const { photos, importedIds } = get();
    const newOnes = photos.filter((p) => !importedIds.has(identityKey(p.filename, p.size)));
    if (newOnes.length === 0) {
      get().toast('No new photos to import', 'info');
      return;
    }
    await get().importPhotos(newOnes);
  },

  async loadProps() {
    if (!camera.connected) return;
    set({ loadingProps: true });
    const props = await camera.readControlProps();
    set({ props, loadingProps: false });
  },

  async changeProp(prop, value) {
    try {
      const fresh = await camera.writeProperty(prop, value);
      if (fresh) {
        const props = get().props.map((p) => (p.code === fresh.code ? fresh : p));
        set({ props });
        await haptics.tap();
      }
    } catch (e) {
      get().toast(e instanceof Error ? e.message : 'This value is not writable', 'error');
      await haptics.warn();
    }
  },

  async capture() {
    try {
      await camera.capture();
      await haptics.success();
      get().toast('Shutter released', 'success');
      window.setTimeout(() => void get().refreshPhotos(), 2000);
    } catch (e) {
      get().toast(e instanceof Error ? e.message : 'Remote capture not supported', 'error');
      await haptics.warn();
    }
  },

  async updateSettings(patch) {
    const settings = { ...get().settings, ...patch };
    set({ settings });
    await saveSettings(settings);
    if (patch.theme) document.documentElement.dataset.theme = patch.theme;
    if (patch.autoImport !== undefined || patch.watchMode !== undefined) {
      const s = get();
      if (s.settings.autoImport && s.settings.watchMode && s.connected) s.startAutoImport();
      else s.stopAutoImport();
    }
  },

  toast(message, kind = 'info') {
    const t: Toast = { id: toastSeq++, message, kind };
    set({ toasts: [...get().toasts, t] });
    window.setTimeout(() => get().dismissToast(t.id), 3200);
  },

  dismissToast(id) {
    set({ toasts: get().toasts.filter((t) => t.id !== id) });
  },

  startAutoImport() {
    if (get().autoImportTimer) return;
    const timer = window.setInterval(async () => {
      const s = get();
      if (!s.connected || s.importing) return;
      await s.refreshPhotos();
      const fresh = get();
      const newOnes = fresh.photos.filter(
        (p) => !fresh.importedIds.has(identityKey(p.filename, p.size)),
      );
      if (newOnes.length > 0) {
        if (fresh.settings.notifyOnImport) void notifyNewPhoto(newOnes[0].filename);
        await fresh.importPhotos(newOnes);
      }
    }, 5000);
    set({ autoImportTimer: timer });
  },

  stopAutoImport() {
    const t = get().autoImportTimer;
    if (t) window.clearInterval(t);
    set({ autoImportTimer: null });
  },

  startKeepAlive() {
    if (get().keepAliveTimer) return;
    // Every ~9s, if the link is connected and NOT busy (importing or mid photo
    // listing), fire one cheap serialized PTP op. It rides the client mutex so
    // it can never overlap real work. This defeats the D5300's idle-session /
    // Wi-Fi drop. On a connection error we disconnect cleanly + toast rather
    // than spinning forever.
    const timer = window.setInterval(async () => {
      const s = get();
      // Short-circuit while busy: a real transaction is already keeping the
      // link warm, and we don't want to pile onto the queue.
      if (!s.connected || s.importing || s.loadingPhotos) return;
      try {
        await camera.keepAlive();
      } catch (e) {
        // A failed keep-alive means the link is gone. Bail cleanly.
        const message = e instanceof Error ? e.message : 'Camera link lost';
        get().toast(`Camera disconnected: ${message}`, 'error');
        await get().disconnect();
      }
    }, 9000);
    set({ keepAliveTimer: timer });
  },

  stopKeepAlive() {
    const t = get().keepAliveTimer;
    if (t) window.clearInterval(t);
    set({ keepAliveTimer: null });
  },

  async checkForUpdates() {
    // Debounce: skip if we checked within the cooldown, or a download is live.
    const now = Date.now();
    if (get().updateApplying) return;
    if (now - lastUpdateCheck < UPDATE_CHECK_COOLDOWN_MS) return;
    lastUpdateCheck = now;
    try {
      const result = await checkForUpdate();
      // Preserve a user's dismissal unless a genuinely newer bundle appears.
      const prev = get();
      const changed = result.latest?.version !== prev.updateLatest?.version;
      set({
        updateStatus: result.status,
        updateLatest: result.latest,
        updateDismissed: changed ? false : prev.updateDismissed,
        updateCurrentVersion: result.current,
        updateNativeVersion: result.native,
        updateLastError: result.error ?? null,
      });
    } catch {
      /* demo-safe: never surface a checker error */
    }
  },

  async checkForUpdatesManual() {
    if (get().updateChecking) return;
    set({ updateChecking: true });
    let result;
    try {
      result = await checkForUpdate();
    } catch (e) {
      const error = e instanceof Error ? e.message : 'update check failed';
      set({ updateChecking: false, updateLastError: error });
      get().toast('Update check failed: ' + error, 'error');
      await haptics.warn();
      return;
    }
    const prev = get();
    const changed = result.latest?.version !== prev.updateLatest?.version;
    set({
      updateStatus: result.status,
      updateLatest: result.latest,
      updateDismissed: changed ? false : prev.updateDismissed,
      updateCurrentVersion: result.current,
      updateNativeVersion: result.native,
      updateLastError: result.error ?? null,
      updateChecking: false,
    });
    // Refresh the debounce clock so the auto-check doesn't immediately re-run.
    lastUpdateCheck = Date.now();

    if (result.error) {
      get().toast('Update check failed: ' + result.error, 'error');
      await haptics.warn();
    } else if (result.status === 'ota-available') {
      get().toast('Update available: v' + (result.latest?.version ?? ''), 'info');
    } else if (result.status === 'native-required') {
      get().toast('A new version needs a full APK update', 'error');
    } else {
      get().toast("You're up to date (v" + result.current + ')', 'success');
    }
  },

  async runUpdate() {
    const latest = get().updateLatest;
    if (!latest || get().updateApplying) return;
    set({ updateApplying: true, updateProgress: 0 });
    const result = await applyUpdate(latest, (percent) => {
      set({ updateProgress: Math.max(0, Math.min(100, Math.round(percent))) });
    });
    if (result.pending) {
      set({ updateApplying: false, updatePending: true, updateProgress: 100 });
      await haptics.success();
      get().toast('Update ready — restart to apply', 'success');
    } else {
      set({ updateApplying: false, updateProgress: 0 });
      await haptics.warn();
      get().toast(result.error || 'Update failed', 'error');
    }
  },

  async applyPendingUpdate() {
    await reloadToApply();
  },

  dismissUpdate() {
    set({ updateDismissed: true });
  },
}));

export { filteredNewCount };
