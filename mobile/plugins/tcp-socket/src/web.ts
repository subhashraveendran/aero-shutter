// Web implementation of the TCP socket plugin.
//
// Real TCP sockets are impossible in a browser, so the web implementation is a
// DEMO-MODE backend: each "connection" is wired to an in-process MockCameraSocket
// that speaks enough PTP/IP for the whole app to run without hardware.

import { WebPlugin } from '@capacitor/core';
import type {
  CloseOptions,
  ConnectOptions,
  ConnectResult,
  CurrentWifiResult,
  JoinWifiOptions,
  JoinWifiResult,
  NetworkCapabilities,
  ScanWifiResult,
  TcpSocketPlugin,
  WifiInfo,
  WriteOptions,
} from './definitions';
import { MockCameraSocket } from './web/mock-camera';

function bytesToB64(bytes: Uint8Array): string {
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk) as unknown as number[]);
  }
  return btoa(binary);
}

function b64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

export class TcpSocketWeb extends WebPlugin implements TcpSocketPlugin {
  /** Signals the app that it is running against the mock backend. */
  static readonly isDemo = true;

  private sockets = new Map<string, MockCameraSocket>();
  private counter = 0;
  /** SSID the demo pretends to be joined to after joinWifi(). */
  private demoWifiSsid: string | null = null;

  async connect(_options: ConnectOptions): Promise<ConnectResult> {
    const socketId = `demo-${++this.counter}`;
    const mock = new MockCameraSocket((event, payload) => {
      if (event === 'data') {
        this.notifyListeners('data', { socketId, dataB64: bytesToB64(payload as Uint8Array) });
      } else if (event === 'closed') {
        this.notifyListeners('closed', { socketId });
      } else if (event === 'error') {
        this.notifyListeners('error', { socketId, message: String(payload) });
      }
    });
    this.sockets.set(socketId, mock);
    // The browser can't split routing; report the socket as unbound.
    return { socketId, networkBinding: 'unsupported' };
  }

  async getWifiInfo(): Promise<WifiInfo> {
    // No Wi-Fi introspection in the browser; discovery falls back to demo host.
    return { gateway: null, ipAddress: null };
  }

  async getNetworkCapabilities(): Promise<NetworkCapabilities> {
    return { isSplitRoutingSupported: false, platform: 'web' };
  }

  // ----- Wi-Fi joining (simulated in the browser demo) --------------------
  // The browser can't touch the OS Wi-Fi stack, so these fake a successful
  // join to "Nikon_WU2_DEMO" and keep the whole flow demoable end-to-end.

  async joinWifi(options: JoinWifiOptions): Promise<JoinWifiResult> {
    const ssid =
      options.ssid ||
      (options.ssidPrefix ? `${options.ssidPrefix}DEMO` : 'Nikon_WU2_DEMO');
    this.demoWifiSsid = ssid;
    return { joined: true, ssid, bound: false };
  }

  async currentWifi(): Promise<CurrentWifiResult> {
    return { ssid: this.demoWifiSsid };
  }

  async scanWifi(): Promise<ScanWifiResult> {
    // Advertise a fake Nikon AP so prefix auto-find is exercisable in demo.
    return { networks: [{ ssid: 'Nikon_WU2_DEMO' }] };
  }

  async leaveWifi(): Promise<void> {
    this.demoWifiSsid = null;
  }

  async write(options: WriteOptions): Promise<void> {
    const mock = this.sockets.get(options.socketId);
    if (!mock) throw new Error(`Unknown socket ${options.socketId}`);
    mock.receive(b64ToBytes(options.dataB64));
  }

  async close(options: CloseOptions): Promise<void> {
    const mock = this.sockets.get(options.socketId);
    if (mock) {
      mock.destroy();
      this.sockets.delete(options.socketId);
    }
  }
}
