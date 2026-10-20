import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

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
  plugins: [svelte()],
  resolve: { conditions: ['browser'] },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'], include: ['src/**/*.test.ts'] },
  server: { host: '127.0.0.1', headers: { 'Content-Security-Policy': contentSecurityPolicy } },
  preview: { host: '127.0.0.1', headers: { 'Content-Security-Policy': contentSecurityPolicy } },
  build: { outDir: 'dist', emptyOutDir: true },
});
