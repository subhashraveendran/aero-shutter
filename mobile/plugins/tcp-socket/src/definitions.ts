import type { PluginListenerHandle } from '@capacitor/core';

export interface ConnectOptions {
  host: string;
  port: number;
  /** Connection timeout in milliseconds. Defaults to 8000. */
  timeoutMs?: number;
}

export interface ConnectResult {
  socketId: string;
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
