import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.aeroshutter.app',
  appName: 'AeroShutter',
  webDir: 'dist',
  backgroundColor: '#0a0a0b',
  plugins: {
    SplashScreen: {
      launchShowDuration: 900,
      backgroundColor: '#0a0a0b',
      showSpinner: false,
      androidSplashResourceName: 'splash',
    },
    StatusBar: {
      style: 'DARK',
      backgroundColor: '#0a0a0b',
      overlaysWebView: false,
    },
    // Self-hosted OTA live updates via the open-source @capgo/capacitor-updater
    // plugin. We do NOT use Capgo Cloud: updates are driven manually from JS
    // (see src/lib/updater.ts) against a manifest on our own GitHub Releases.
    CapacitorUpdater: {
      // We drive the whole check/download/set flow ourselves.
      autoUpdate: false,
      // Drop stale downloaded bundles whenever a new native APK is installed.
      resetWhenUpdate: true,
      // We call notifyAppReady() from JS on startup; if a freshly-set bundle
      // never reaches that call within the timeout, the plugin auto-rolls back
      // to the last good bundle. This is our bad-bundle safety net.
      appReadyTimeout: 10000,
    },
  },
  ios: {
    contentInset: 'never',
  },
  server: {
    androidScheme: 'https',
  },
};

export default config;
