import { readFileSync } from 'node:fs';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

const appVersion = readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim();
if (!appVersion) throw new Error('VERSION must contain a build version');

const contentSecurityPolicy = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "connect-src 'self'",
  "font-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "frame-src 'none'",
  "frame-ancestors 'none'",
].join('; ');

export default defineConfig({
  define: { __APP_VERSION__: JSON.stringify(appVersion) },
  plugins: [svelte()],
  resolve: { conditions: ['browser'] },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov', 'json-summary'],
      include: ['src/App.svelte', 'src/components/*.svelte', 'src/lib/bridge.ts', 'src/lib/format.ts'],
      exclude: ['src/**/*.test.ts'],
      thresholds: { lines: 75 },
    },
  },
  server: { host: '127.0.0.1', headers: { 'Content-Security-Policy': contentSecurityPolicy } },
  preview: { host: '127.0.0.1', headers: { 'Content-Security-Policy': contentSecurityPolicy } },
  build: { outDir: 'dist', emptyOutDir: true },
});
