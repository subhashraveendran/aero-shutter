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
  NetworkCapabilities,
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
