// Network-binding capability + status helpers for the UI.
//
// Wraps the tcp-socket plugin's getNetworkCapabilities() and turns a raw
// `networkBinding` value into a short human-readable status the connect /
// settings screens can display.

import { isDemoMode, TcpSocket } from '@aero-shutter/tcp-socket';
import type { NetworkBinding, NetworkCapabilities } from '@aero-shutter/tcp-socket';

/** Ask the platform whether it can split camera Wi-Fi from cellular internet. */
export async function getNetworkCapabilities(): Promise<NetworkCapabilities> {
  if (isDemoMode()) return { isSplitRoutingSupported: false, platform: 'web' };
  const anyPlugin = TcpSocket as unknown as {
    getNetworkCapabilities?: () => Promise<NetworkCapabilities>;
  };
  if (typeof anyPlugin.getNetworkCapabilities !== 'function') {
    return { isSplitRoutingSupported: false, platform: 'web' };
  }
  try {
    return await anyPlugin.getNetworkCapabilities();
  } catch {
    return { isSplitRoutingSupported: false, platform: 'web' };
  }
}

/** Short label for a binding, e.g. to show under the connection status. */
export function bindingStatusLabel(binding: NetworkBinding | string | null): string {
  switch (binding) {
    case 'wifi-bound':
      return 'Using mobile data for internet';
    case 'wifi-pinned':
      return 'Camera pinned to Wi-Fi';
    case 'default':
    case 'unsupported':
    default:
      return '';
  }
}
