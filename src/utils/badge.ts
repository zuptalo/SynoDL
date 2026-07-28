/**
 * App-icon badge counter, shared between the service worker and the page.
 *
 * The badge should count PUSH notifications received while the app is closed and
 * reset to zero the moment the app is opened/foregrounded — so it always reflects
 * "new things since you last looked". The SW is stateless between push events, so
 * the running count is kept in its own tiny IndexedDB (separate from the app's
 * main DB to avoid coupling the worker to that schema). `navigator` resolves to
 * the WorkerNavigator inside the SW and window.navigator on the page; both expose
 * setAppBadge/clearAppBadge where supported (a no-op elsewhere).
 */
const DB = 'synodl-badge';
const STORE = 'kv';
const KEY = 'count';

type BadgeNav = {
  setAppBadge?: (n?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
};

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => req.result.createObjectStore(STORE);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function getCount(): Promise<number> {
  return openDB().then(
    (db) =>
      new Promise<number>((resolve) => {
        const r = db.transaction(STORE).objectStore(STORE).get(KEY);
        r.onsuccess = () => resolve(typeof r.result === 'number' ? r.result : 0);
        r.onerror = () => resolve(0);
      }),
  );
}

function setCount(n: number): Promise<void> {
  return openDB().then(
    (db) =>
      new Promise<void>((resolve) => {
        const tx = db.transaction(STORE, 'readwrite');
        tx.objectStore(STORE).put(n, KEY);
        tx.oncomplete = () => resolve();
        tx.onerror = () => resolve();
      }),
  );
}

/** SW: a background push arrived — grow the badge by one. */
export async function incrementBadge(): Promise<void> {
  try {
    const next = (await getCount()) + 1;
    await setCount(next);
    await (navigator as Navigator & BadgeNav).setAppBadge?.(next);
  } catch {
    /* best-effort — the badge is a nicety, never fail a push over it */
  }
}

/** Page: the app is open/foregrounded — clear the icon badge and the count. */
export async function resetBadge(): Promise<void> {
  try {
    await setCount(0);
    await (navigator as Navigator & BadgeNav).clearAppBadge?.();
  } catch {
    /* best-effort */
  }
}
