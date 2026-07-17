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
}

export const DEFAULT_SETTINGS: Settings = {
  destination: 'files',
  autoImport: false,
  watchMode: true,
  cameraIp: '192.168.1.1',
  keepAwake: true,
  theme: 'dark',
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
