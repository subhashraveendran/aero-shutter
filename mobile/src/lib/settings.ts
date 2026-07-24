// Persistent settings via Capacitor Preferences (falls back to localStorage on web).

import { Preferences } from '@capacitor/preferences';

export type Destination = 'gallery' | 'files' | 'off';
export type Theme = 'dark' | 'light';

export interface Settings {
  destination: Destination;
  autoImport: boolean;
  watchMode: boolean;
  cameraIp: string;
  keepAwake: boolean;
  theme: Theme;
  /**
   * Android only: bind the camera socket to the Wi-Fi network so the rest of
   * the device keeps using cellular for internet while joined to the camera's
   * no-internet AP. No effect on iOS / web.
   */
  keepInternetOnCellular: boolean;
  /** Fire local notifications on import-complete and new-shot events. */
  notifyOnImport: boolean;
  /** Keep-alive ping interval in ms while connected + idle (fix #1). */
  keepAliveIntervalMs: number;
  /** Auto-reconnect with exponential backoff after a disconnect (fix #5). */
  autoReconnect: boolean;
}

export const DEFAULT_SETTINGS: Settings = {
  destination: 'files',
  autoImport: false,
  watchMode: true,
  cameraIp: '192.168.1.1',
  keepAwake: true,
  theme: 'dark',
  keepInternetOnCellular: true,
  notifyOnImport: true,
  keepAliveIntervalMs: 9000,
  autoReconnect: true,
};

const KEY = 'aero-shutter.settings';

export async function loadSettings(): Promise<Settings> {
  try {
    const { value } = await Preferences.get({ key: KEY });
    if (!value) return { ...DEFAULT_SETTINGS };
    return { ...DEFAULT_SETTINGS, ...(JSON.parse(value) as Partial<Settings>) };
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export async function saveSettings(settings: Settings): Promise<void> {
  await Preferences.set({ key: KEY, value: JSON.stringify(settings) });
}
