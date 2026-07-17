// Zero-config camera discovery.
//
// When a phone joins a Nikon camera's own Wi-Fi access point, the camera acts
// as the network gateway / DHCP server, so it is reachable at a predictable
// address. We build a small list of candidate IPs to probe concurrently:
//
//   1. The live Wi-Fi gateway / DHCP-server address (native only, via the
//      tcp-socket plugin's getWifiInfo()) and the /24 ".1" of the phone's own
//      Wi-Fi IP — this is the strongest signal.
//   2. A short static list of the addresses Nikon cameras hand out on their
//      built-in APs (the D5300 uses 192.168.1.1).
//
// In demo mode discovery returns the mock loopback address so the browser
// build connects to the in-process MockCameraSocket.

import { isDemoMode, TcpSocket } from '@aero-shutter/tcp-socket';

/** Loopback address the browser demo mock answers on (host is irrelevant). */
export const DEMO_HOST = '127.0.0.1';

/**
 * Common Nikon Wi-Fi AP gateway addresses, in rough order of likelihood.
 * The D5300 (and most SnapBridge / WMU bodies) sit at 192.168.1.1; a few
 * older/alternate firmwares use the other subnets.
 */
export const NIKON_AP_CANDIDATES: readonly string[] = [
  '192.168.1.1',
  '192.168.0.1',
  '192.168.1.2',
  '10.0.0.1',
  '192.168.4.1',
];

/** Derive the ".1" gateway guess for a /24 from an interface IPv4 address. */
export function gatewayGuessFromIp(ip: string | null | undefined): string | null {
  if (!ip) return null;
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip.trim());
  if (!m) return null;
  const octets = [m[1], m[2], m[3], m[4]].map((o) => Number(o));
  if (octets.some((o) => o < 0 || o > 255)) return null;
  // Skip loopback / link-local / unspecified.
  if (octets[0] === 127 || octets[0] === 0) return null;
  if (octets[0] === 169 && octets[1] === 254) return null;
  return `${octets[0]}.${octets[1]}.${octets[2]}.1`;
}

/** De-duplicate while preserving order. */
function dedupe(list: (string | null | undefined)[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of list) {
    if (!item) continue;
    if (seen.has(item)) continue;
    seen.add(item);
    out.push(item);
  }
  return out;
}

export interface WifiInfo {
  /** Gateway / DHCP-server address of the current Wi-Fi network, if known. */
  gateway: string | null;
  /** The phone's own IPv4 address on the Wi-Fi interface, if known. */
  ipAddress: string | null;
}

/**
 * Read the current Wi-Fi gateway from the native plugin. Returns an empty
 * WifiInfo on web/demo or when the platform can't tell us.
 */
export async function readWifiInfo(): Promise<WifiInfo> {
  const empty: WifiInfo = { gateway: null, ipAddress: null };
  if (isDemoMode()) return empty;
  const anyPlugin = TcpSocket as unknown as {
    getWifiInfo?: () => Promise<{ gateway?: string | null; ipAddress?: string | null }>;
  };
  if (typeof anyPlugin.getWifiInfo !== 'function') return empty;
  try {
    const info = await anyPlugin.getWifiInfo();
    return {
      gateway: info.gateway && info.gateway !== '0.0.0.0' ? info.gateway : null,
      ipAddress: info.ipAddress && info.ipAddress !== '0.0.0.0' ? info.ipAddress : null,
    };
  } catch {
    return empty;
  }
}

/**
 * Build the ordered list of candidate camera IPs to probe.
 *
 * A caller-supplied `preferred` address (e.g. the last-used cameraIp from
 * settings) is tried first. In demo mode the single loopback host is returned.
 */
export async function candidateHosts(preferred?: string | null): Promise<string[]> {
  if (isDemoMode()) return [DEMO_HOST];

  const info = await readWifiInfo();
  return dedupe([
    preferred,
    info.gateway,
    gatewayGuessFromIp(info.ipAddress),
    ...NIKON_AP_CANDIDATES,
  ]);
}
