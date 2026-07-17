import { registerPlugin } from '@capacitor/core';
import type { TcpSocketPlugin } from './definitions';

// The native implementations live in android/ and ios/. On the web the plugin
// falls back to the demo-mode implementation registered lazily below.
const TcpSocket = registerPlugin<TcpSocketPlugin>('TcpSocket', {
  web: () => import('./web').then((m) => new m.TcpSocketWeb()),
});

/**
 * True when the app is running against the browser demo backend (no native
 * TCP available). Used to show the "Demo mode" badge.
 */
export function isDemoMode(): boolean {
  // Capacitor sets a native bridge on window when running in a native shell.
  const cap = (globalThis as { Capacitor?: { isNativePlatform?: () => boolean } }).Capacitor;
  return !(cap?.isNativePlatform?.() ?? false);
}

export * from './definitions';
export { TcpSocket };
