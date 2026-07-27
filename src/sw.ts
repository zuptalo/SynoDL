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

// Web Push (spec 0003): the server sends a JSON payload {title, body}. We only
// show a SYSTEM notification when the user isn't actively looking at the app
// (spec 1013): if a window is in the foreground we hand the message to the page
// instead, which shows an in-app notice (or nothing, on the Tasks tab where the
// change is already visible live).
self.addEventListener('push', (event) => {
  let title = 'SynoDL';
  let body = 'A download update is available.';
  try {
    const data = event.data?.json() as { title?: string; body?: string } | undefined;
    if (data?.title) title = data.title;
    if (data?.body) body = data.body;
  } catch {
    if (event.data) body = event.data.text();
  }

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      const foreground = clients.some((c) => c.visibilityState === 'visible');
      if (foreground) {
        // App is open in front — let the page decide (in-app toast off the Tasks
        // tab, nothing on it). No OS notification, no badge.
        for (const c of clients) {
          c.postMessage({ type: 'push-notification', title, body });
        }
        return undefined;
      }
      // Backgrounded / closed — show the OS notification + set the app badge.
      const nav = self.navigator as Navigator & { setAppBadge?: (n?: number) => Promise<void> };
      return Promise.all([
        self.registration.showNotification(title, {
          body,
          icon: '/pwa-192x192.png',
          badge: '/pwa-192x192.png',
          tag: 'synodl-download',
        }),
        nav.setAppBadge ? nav.setAppBadge().catch(() => undefined) : Promise.resolve(),
      ]);
    }),
  );
});

// Tapping a notification focuses an open SynoDL tab or opens one.
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const c of clients) {
        if ('focus' in c) return c.focus();
      }
      return self.clients.openWindow('/tabs/tasks');
    }),
  );
});
