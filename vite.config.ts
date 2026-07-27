import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { VitePWA } from 'vite-plugin-pwa';
import path from 'node:path';
import { readFileSync } from 'node:fs';

// App version, exposed to the client as the compile-time constant __APP_VERSION__
// (shown in Settings and compared against the server's /v1/config version to detect
// a new deploy). Prefer SYNODL_VERSION when the build sets it (the Docker image
// stamps the SAME value into both this and the Go binary's main.version, so the UI
// shows the true DEPLOYED version), falling back to package.json for local/dev
// builds. Read package.json via fs rather than process.env.npm_package_version so
// it works under `npx vite` too.
const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf-8')) as {
  version: string;
};
const appVersion = process.env.SYNODL_VERSION || pkg.version;

// This build's release notes (changes since the last release tag), exposed as the
// compile-time constant __RELEASE_NOTES__ and used as the "running" side of the
// What's-new update delta. The Docker build passes the SAME JSON into both this and
// the Go binary (served from /v1/config), produced by scripts/release-notes.sh.
// Defaults to an empty list for local/dev builds that don't set SYNODL_RELEASE_NOTES.
let releaseNotes: unknown = [];
try {
  releaseNotes = JSON.parse(process.env.SYNODL_RELEASE_NOTES || '[]');
} catch {
  releaseNotes = [];
}

// Backend the dev server proxies /v1 + /healthz to. Defaults to local synodl on
// :8080; the e2e harness overrides it to its isolated test backend.
const proxyTarget = process.env.SYNODL_PROXY_TARGET || 'http://localhost:8080';

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
    __RELEASE_NOTES__: JSON.stringify(releaseNotes),
  },
  server: {
    host: true, // listen on 0.0.0.0 so the LAN address is reachable from a phone
    port: 5173,
    // Only watch app source. Without this, Vite watches the whole repo and a full
    // page reload fires whenever the Go backend rebuilds (server/tmp), e2e
    // artifacts change, etc.
    watch: {
      ignored: [
        '**/server/**',
        '**/test-results/**',
        '**/playwright-report/**',
        '**/dist/**',
        '**/.git/**',
        '**/.tmp/**',
      ],
    },
    // Proxy the backend through the dev server so the client can use same-origin
    // URLs (/v1/...). The target is overridable (SYNODL_PROXY_TARGET) so the e2e
    // harness can point a test frontend at an isolated test backend.
    proxy: {
      '/v1': { target: proxyTarget, changeOrigin: true },
      '/healthz': { target: proxyTarget, changeOrigin: true },
    },
  },
  preview: {
    host: true,
    port: 5173,
  },
  plugins: [
    vue(),
    VitePWA({
      // 'prompt' (not autoUpdate): a new deploy must not silently reload the page
      // out from under the user (constitution Principle V). The app surfaces a
      // prompt naming the new version and applies it only when the user accepts
      // (see useAppUpdate + sw.ts SKIP_WAITING).
      registerType: 'prompt',
      // Custom service worker (src/sw.ts) so precaching stays explicit and a
      // future notifications spec has a place to land. esbuild-compiled by the
      // plugin.
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      includeAssets: ['favicon.ico', 'favicon.svg', 'apple-touch-icon.png'],
      // Serve the manifest + service worker in `vite dev` too, so installing
      // to the home screen from the dev server behaves like the real PWA
      // (proper scope/start_url). type: 'module' lets the dev SW use ES imports.
      devOptions: { enabled: true, type: 'module' },
      manifest: {
        name: 'SynoDL',
        short_name: 'SynoDL',
        description: 'Mobile client for Synology Download Station',
        theme_color: '#f97316',
        background_color: '#0a0a0a',
        display: 'standalone',
        id: '/',
        start_url: '/',
        scope: '/',
        icons: [
          { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
          { src: 'pwa-maskable-192x192.png', sizes: '192x192', type: 'image/png', purpose: 'maskable' },
          { src: 'pwa-maskable-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
    }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
});
