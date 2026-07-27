/// <reference types="vitest" />
// Vitest config for client-side unit tests. Kept separate from vite.config.ts on
// purpose: the unit tests don't need the PWA plugin / dev-server / proxy machinery,
// and loading the VitePWA plugin under the test runner is pure overhead.
import { defineConfig } from 'vitest/config';
import path from 'node:path';

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    // The gated modules are pure (or run against fake-indexeddb), so the fast
    // Node environment is enough. Add happy-dom + @vue/test-utils here if/when
    // component tests land.
    environment: 'node',
    include: ['src/**/*.test.ts'], // unit tests; e2e/*.spec.ts is Playwright's (testDir: ./e2e)
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary', 'html'],
      // Gate the pure modules we exercise directly — an honest number for the
      // tested core, not a meaningless whole-app percentage. Extend this list as
      // specs land new pure modules with their unit tests (constitution
      // Principle II: coverage floors are a ratchet — add, never remove).
      include: [
        'src/db/idb.ts',
        'src/utils/format.ts',
        // Pure MVP logic (spec 0001): sort/filter, URL extraction, error copy.
        'src/services/task-sort.ts',
        'src/services/url-detect.ts',
        'src/services/syno-errors.ts',
      ],
      thresholds: { lines: 80, functions: 80, statements: 80, branches: 80 },
    },
  },
});
