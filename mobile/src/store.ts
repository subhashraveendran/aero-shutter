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
import * as haptics from './lib/haptics';

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

  settings: Settings;

  photos: Photo[];
  loadingPhotos: boolean;
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

  // actions
  init: () => Promise<void>;
  navigate: (screen: Screen) => void;
  connect: (ip?: string) => Promise<void>;
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
}

let toastSeq = 1;
let importCancelled = false;

function filteredNewCount(photos: Photo[], imported: Set<string>): number {
  return photos.filter((p) => !imported.has(identityKey(p.filename, p.size))).length;
}

export const useStore = create<AppState>((set, get) => ({
  demo: isDemoMode(),
  screen: 'connect',
  connected: false,
  connecting: false,
  connectError: null,
  cameraModel: '',

  settings: { ...DEFAULT_SETTINGS },

  photos: [],
  loadingPhotos: false,
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

  async init() {
    const settings = await loadSettings();
    const importedIds = await allImportedIds();
    const importedTodayCount = await countImportedToday();
    document.documentElement.dataset.theme = settings.theme;
    set({ settings, importedIds, importedTodayCount });
  },

  navigate(screen) {
    set({ screen });
  },

  async connect(ip) {
    const host = ip ?? get().settings.cameraIp;
    set({ connecting: true, connectError: null });
    try {
      await camera.connect(host);
      set({ connected: true, connecting: false, cameraModel: camera.cameraModel, screen: 'gallery' });
      await haptics.success();
      await get().refreshPhotos();
      if (get().settings.autoImport && get().settings.watchMode) get().startAutoImport();
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Could not reach the camera';
      set({ connecting: false, connectError: message });
      await haptics.warn();
    }
  },

  async enterDemo() {
    // Demo mode uses the same code path; the host is irrelevant to the mock.
    await get().connect('127.0.0.1');
  },

  async disconnect() {
    const t = get().autoImportTimer;
    if (t) window.clearInterval(t);
    await camera.disconnect();
    set({
      connected: false,
      photos: [],
      selection: new Set(),
      props: [],
      screen: 'connect',
      autoImportTimer: null,
    });
  },

  async refreshPhotos() {
    if (!camera.connected) return;
    set({ loadingPhotos: true });
    try {
      const photos = await camera.listPhotos();
      set({ photos, loadingPhotos: false });
    } catch (e) {
      set({ loadingPhotos: false });
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
      if (newOnes.length > 0) await fresh.importPhotos(newOnes);
    }, 5000);
    set({ autoImportTimer: timer });
  },

  stopAutoImport() {
    const t = get().autoImportTimer;
    if (t) window.clearInterval(t);
    set({ autoImportTimer: null });
  },
}));

export { filteredNewCount };
