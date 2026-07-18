/// <reference types="vite/client" />

// Injected at build time by Vite's `define` (see vite.config.ts). Holds the
// shipped app version from mobile/package.json, used as the OTA updater's
// fallback bundle version when the Capgo plugin has no downloaded bundle yet.
declare const __APP_VERSION__: string;
