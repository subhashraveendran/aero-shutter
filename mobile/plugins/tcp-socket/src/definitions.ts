import type { PluginListenerHandle } from '@capacitor/core';

export interface ConnectOptions {
  host: string;
  port: number;
  /** Connection timeout in milliseconds. Defaults to 8000. */
  timeoutMs?: number;
  /**
   * Android only: bind this socket to the Wi-Fi transport (without requiring
   * NET_CAPABILITY_INTERNET) so camera traffic goes over Wi-Fi while the rest
   * of the device keeps using cellular for internet. Defaults to true.
   * Ignored on iOS (the socket is pinned to Wi-Fi but the OS still routes app
   * internet however it likes) and on web.
   */
  bindWifi?: boolean;
}

export type NetworkBinding = 'wifi-bound' | 'wifi-pinned' | 'default' | 'unsupported';

export interface ConnectResult {
  socketId: string;
  /**
   * How this socket was bound to the network:
   *  - 'wifi-bound'  Android: socket lives on a dedicated Wi-Fi Network while
   *                  the process default (app internet) stays on cellular.
   *  - 'wifi-pinned' iOS: connection required the Wi-Fi interface, but the OS
   *                  still owns internet routing (no cellular split).
   *  - 'default'     bindWifi was false / no split applied.
   *  - 'unsupported' platform can't split routing (web/demo).
   */
  networkBinding?: NetworkBinding;
}

export interface WifiInfo {
  /** Gateway / DHCP-server address of the current Wi-Fi network, or null. */
  gateway: string | null;
  /** The phone's own IPv4 on the Wi-Fi interface, or null. */
  ipAddress: string | null;
}

export interface NetworkCapabilities {
  /**
   * True on Android, where a single socket can be bound to Wi-Fi while the
   * device keeps cellular internet. False on iOS/web.
   */
  isSplitRoutingSupported: boolean;
  platform: 'android' | 'ios' | 'web';
}

export interface WriteOptions {
  socketId: string;
  /** Payload encoded as base64 for binary safety. */
  dataB64: string;
}

export interface ReadOptions {
  socketId: string;
}

export interface CloseOptions {
  socketId: string;
}

export interface DataEvent {
  socketId: string;
  /** Received bytes, base64-encoded. */
  dataB64: string;
}

export interface ClosedEvent {
  socketId: string;
}

export interface ErrorEvent {
  socketId: string;
  message: string;
}

export interface TcpSocketPlugin {
  connect(options: ConnectOptions): Promise<ConnectResult>;
  write(options: WriteOptions): Promise<void>;
  close(options: CloseOptions): Promise<void>;

  /** Read the current Wi-Fi gateway / DHCP-server address (native only). */
  getWifiInfo(): Promise<WifiInfo>;

  /** Report whether this platform can split camera Wi-Fi from internet. */
  getNetworkCapabilities(): Promise<NetworkCapabilities>;

  addListener(
    eventName: 'data',
    listenerFunc: (event: DataEvent) => void,
  ): Promise<PluginListenerHandle>;
  addListener(
    eventName: 'closed',
    listenerFunc: (event: ClosedEvent) => void,
  ): Promise<PluginListenerHandle>;
  addListener(
    eventName: 'error',
    listenerFunc: (event: ErrorEvent) => void,
  ): Promise<PluginListenerHandle>;

  removeAllListeners(): Promise<void>;
}
