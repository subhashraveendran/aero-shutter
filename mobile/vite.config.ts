import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';

// The shipped app version, injected as __APP_VERSION__ so the OTA updater has a
// sane fallback bundle version when the Capgo plugin has no downloaded bundle
// yet (fresh APK install). In CI we stamp the real release tag via the
// APP_VERSION env var; for local/dev builds we fall back to package.json.
const pkg = JSON.parse(readFileSync(resolve(__dirname, 'package.json'), 'utf-8')) as {
  version: string;
};
const appVersion = process.env.APP_VERSION?.trim() || pkg.version;

// OTA builds must resolve assets relative to index.html ('./assets/...') so the
// bundle works when served from an arbitrary path inside the native webview.
const isOtaBuild = !!process.env.OTA_BUILD;

// base is '/' for local dev and native Capacitor builds, and can be
// overridden via VITE_BASE (e.g. '/aero-shutter/') for GitHub Pages so
// that bundled asset URLs resolve under the project pages subpath. For OTA
// bundles we force a relative base regardless of VITE_BASE.
export default defineConfig({
  base: isOtaBuild ? './' : process.env.VITE_BASE || '/',
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@aero-shutter/tcp-socket': resolve(__dirname, 'plugins/tcp-socket/src/index.ts'),
    },
  },
  server: {
    port: 5173,
    host: true,
  },
  build: {
    outDir: isOtaBuild ? 'dist-ota' : 'dist',
    sourcemap: true,
    target: 'es2020',
  },
});
