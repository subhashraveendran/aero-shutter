import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';

// base is '/' for local dev and native Capacitor builds, and can be
// overridden via VITE_BASE (e.g. '/aero-shutter/') for GitHub Pages so
// that bundled asset URLs resolve under the project pages subpath.
export default defineConfig({
  base: process.env.VITE_BASE || '/',
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
    outDir: 'dist',
    sourcemap: true,
    target: 'es2020',
  },
});
