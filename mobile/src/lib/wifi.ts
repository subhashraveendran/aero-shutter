// In-app Wi-Fi joining wrapper over the tcp-socket plugin.
//
// Lets AeroShutter join the Nikon camera's own access point ("Nikon_WU2_…")
// from inside the app instead of forcing the user out to system Wi-Fi settings.
//
// The native plugin exposes joinWifi / currentWifi / scanWifi / leaveWifi (see
// plugins/tcp-socket). On the web/demo backend those calls are simulated so the
// entire flow — join → auto-connect → mock gallery — works in a browser.

import { isDemoMode, TcpSocket } from '@aero-shutter/tcp-socket';

/** Default SSID prefix Nikon Wi-Fi bodies broadcast (D5300 / WMU / WU-1a…). */
export const NIKON_SSID_PREFIX = 'Nikon_WU2_';

export interface JoinWifiOptions {
  ssid?: string;
  password?: string;
  ssidPrefix?: string;
}

export interface JoinWifiResult {
  joined: boolean;
  ssid: string;
  bound: boolean;
}

export interface WifiNetwork {
  ssid: string;
}

// The plugin's Wi-Fi methods are optional at the type level so older native
// builds (without them) don't hard-crash; we feature-detect before calling.
type WifiPlugin = {
  joinWifi?: (o: JoinWifiOptions) => Promise<JoinWifiResult>;
  currentWifi?: () => Promise<{ ssid: string | null }>;
  scanWifi?: () => Promise<{ networks: WifiNetwork[] }>;
  leaveWifi?: () => Promise<void>;
};

// Resolve lazily on each call so tests can swap the plugin stub after import,
// and so a native build that adds the methods later is still picked up.
function wifiPlugin(): WifiPlugin {
  return TcpSocket as unknown as WifiPlugin;
}

/**
 * Pure helper: pick the first SSID in `networks` that starts with `prefix`.
 * Case-insensitive so "nikon_wu2_" matches "Nikon_WU2_1234". Returns null when
 * nothing matches. Exported for direct unit testing.
 */
export function matchSsidByPrefix(
  networks: readonly WifiNetwork[] | readonly string[],
  prefix: string,
): string | null {
  const p = prefix.toLowerCase();
  for (const n of networks) {
    const ssid = typeof n === 'string' ? n : n.ssid;
    if (ssid && ssid.toLowerCase().startsWith(p)) return ssid;
  }
  return null;
}

/** True when in-app Wi-Fi joining is even worth trying on this platform. */
export function isWifiJoinSupported(): boolean {
  if (isDemoMode()) return true;
  return typeof wifiPlugin().joinWifi === 'function';
}

/**
 * Join a Wi-Fi network. Supply an exact `ssid`, or an `ssidPrefix` to join the
 * first visible match (the native layer resolves the prefix where the OS
 * allows). Falls back to a simulated success in demo mode.
 */
export async function joinWifi(options: JoinWifiOptions): Promise<JoinWifiResult> {
  const fallbackSsid =
    options.ssid || (options.ssidPrefix ? `${options.ssidPrefix}…` : '');
  if (isDemoMode()) {
    const ssid =
      options.ssid ||
      (options.ssidPrefix ? `${options.ssidPrefix}DEMO` : `${NIKON_SSID_PREFIX}DEMO`);
    return { joined: true, ssid, bound: false };
  }
  const plugin = wifiPlugin();
  if (typeof plugin.joinWifi !== 'function') {
    return { joined: false, ssid: fallbackSsid, bound: false };
  }
  try {
    return await plugin.joinWifi(options);
  } catch {
    return { joined: false, ssid: fallbackSsid, bound: false };
  }
}

/** Best-effort readout of the currently-joined SSID (null if unknown/denied). */
export async function currentWifi(): Promise<string | null> {
  const plugin = wifiPlugin();
  if (isDemoMode()) {
    try {
      const res = await plugin.currentWifi?.();
      return res?.ssid ?? null;
    } catch {
      return null;
    }
  }
  if (typeof plugin.currentWifi !== 'function') return null;
  try {
    const res = await plugin.currentWifi();
    return res.ssid ?? null;
  } catch {
    return null;
  }
}

/** Best-effort scan of visible SSIDs (may be empty if the OS denies it). */
export async function scanWifi(): Promise<WifiNetwork[]> {
  const plugin = wifiPlugin();
  if (typeof plugin.scanWifi !== 'function') return [];
  try {
    const res = await plugin.scanWifi();
    return res?.networks ?? [];
  } catch {
    return [];
  }
}

/** Scan and return the first SSID matching `prefix`, or null. */
export async function findSsidByPrefix(prefix: string): Promise<string | null> {
  const networks = await scanWifi();
  return matchSsidByPrefix(networks, prefix);
}

/** Release any app-requested Wi-Fi binding / configuration. */
export async function leaveWifi(): Promise<void> {
  const plugin = wifiPlugin();
  if (typeof plugin.leaveWifi !== 'function') return;
  try {
    await plugin.leaveWifi();
  } catch {
    /* best effort */
  }
}
