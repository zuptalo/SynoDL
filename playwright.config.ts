import { defineConfig, devices } from '@playwright/test';

/**
 * e2e config for SynoDL.
 *
 * global-setup builds and starts an isolated test stack — the mock DSM
 * (synomock :8091) and a test synodl (:8081) pointed at it; the webServer below
 * serves a test vite on :5174 proxied at that backend. No real NAS anywhere.
 * Serial, single worker — the specs share one mock whose state they seed/reset.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 20_000 },
  // CI runs on a shared 2-core runner where a timing-sensitive test can lose
  // any given run under load. Retries absorb that flake without masking a real
  // break (a genuine failure still fails every attempt). No retries locally so
  // a real failure surfaces immediately. forbidOnly stops a stray test.only
  // from silently shrinking the CI suite.
  retries: process.env.CI ? 2 : 0,
  forbidOnly: !!process.env.CI,
  reporter: [['list']],
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  use: {
    baseURL: 'http://localhost:5174',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'npx vite --port 5174 --strictPort',
    url: 'http://localhost:5174',
    reuseExistingServer: false,
    timeout: 60_000,
    env: { SYNODL_PROXY_TARGET: `http://localhost:${process.env.SYNODL_E2E_PORT || 8081}` },
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // CHROMIUM_PATH points at a system Chromium when the environment can't
        // download Playwright's own build (e.g. a sandboxed dev container). CI
        // leaves it unset and uses `npx playwright install chromium`.
        ...(process.env.CHROMIUM_PATH
          ? { launchOptions: { executablePath: process.env.CHROMIUM_PATH } }
          : {}),
      },
    },
  ],
});
