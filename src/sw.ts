/// <reference lib="webworker" />
/**
 * Custom service worker (injectManifest): explicit app-shell precaching plus
 * the SKIP_WAITING handshake behind the prompt-based update flow. Kept minimal
 * on purpose — a future notifications spec extends this file.
 */
import { precacheAndRoute, cleanupOutdatedCaches } from 'workbox-precaching';

declare let self: ServiceWorkerGlobalScope;

// The build injects the precache manifest here.
precacheAndRoute(self.__WB_MANIFEST);
cleanupOutdatedCaches();

// The page posts SKIP_WAITING only after the user accepts the update prompt
// (useAppUpdate). Never call skipWaiting() unconditionally — that would be a
// silent reload (constitution Principle V).
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    void self.skipWaiting();
  }
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});
