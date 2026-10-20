import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  reporter: 'list',
  timeout: 15_000,
  use: { baseURL: 'http://127.0.0.1:4179', ...devices['Desktop Chrome'], viewport: { width: 460, height: 760 } },
  webServer: {
    command:
      'VITE_TEST_FIXTURES=true pnpm exec vite build && VITE_TEST_FIXTURES=true pnpm exec vite preview --host 127.0.0.1 --port 4179 --strictPort',
    url: 'http://127.0.0.1:4179',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
