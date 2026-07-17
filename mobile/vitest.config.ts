import { defineConfig } from 'vitest/config';
import { resolve } from 'node:path';

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@aero-shutter/tcp-socket': resolve(__dirname, 'plugins/tcp-socket/src/index.ts'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx', 'plugins/**/*.test.ts'],
    testTimeout: 20000,
  },
});
