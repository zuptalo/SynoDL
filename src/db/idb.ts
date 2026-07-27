/**
 * Minimal promise-based IndexedDB wrapper with a change-notification bus.
 * No external dependencies. The bus is what makes queries reactive: any write
 * notifies subscribers of the affected store, and `useLiveQuery` re-runs.
 *
 * App data only lives here (constitution Principle IV): the NAS session,
 * settings, and (future specs) browser favorites/history. Download tasks are
 * never persisted — the NAS is their source of truth.
 */

export const STORES = [
  // Single-row-ish key/value records: the saved session, filter state, theme…
  'settings',
  // Reserved for the in-app browser spec: favorite sites + visit history.
  'favorites',
  'history',
] as const;
export type StoreName = (typeof STORES)[number];

const DB_NAME = 'synodl';
// v1: initial schema — all three stores, keyPath 'id'. Adding or altering a
// store MUST bump this and extend the upgrade below with a forward migration
// that preserves existing data (constitution Principle IV).
const DB_VERSION = 1;

let dbPromise: Promise<IDBDatabase> | null = null;

export function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      for (const name of STORES) {
        if (!db.objectStoreNames.contains(name)) {
          db.createObjectStore(name, { keyPath: 'id' });
        }
      }
    };
    req.onsuccess = () => {
      const db = req.result;
      // If another connection (another tab, or a "clear site data") needs to
      // upgrade/delete the DB, close ours so it can proceed and drop the cached
      // handle so the next operation reopens a fresh connection.
      db.onversionchange = () => {
        db.close();
        dbPromise = null;
      };
      db.onclose = () => {
        dbPromise = null;
      };
      resolve(db);
    };
    req.onerror = () => reject(req.error ?? new Error('indexedDB.open failed'));
  });
  dbPromise.catch(() => {
    dbPromise = null; // allow a later retry instead of caching the failure
  });
  return dbPromise;
}

function promisify<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function store(name: StoreName, mode: IDBTransactionMode): Promise<IDBObjectStore> {
  const db = await openDB();
  return db.transaction(name, mode).objectStore(name);
}

export async function get<T>(name: StoreName, key: IDBValidKey): Promise<T | undefined> {
  return promisify((await store(name, 'readonly')).get(key) as IDBRequest<T | undefined>);
}

export async function getAll<T>(name: StoreName): Promise<T[]> {
  return promisify((await store(name, 'readonly')).getAll() as IDBRequest<T[]>);
}

export async function put<T extends { id: IDBValidKey }>(name: StoreName, value: T): Promise<void> {
  await promisify((await store(name, 'readwrite')).put(value));
  notify(name);
}

export async function del(name: StoreName, key: IDBValidKey): Promise<void> {
  await promisify((await store(name, 'readwrite')).delete(key));
  notify(name);
}

export async function clear(name: StoreName): Promise<void> {
  await promisify((await store(name, 'readwrite')).clear());
  notify(name);
}

// ---- change-notification bus --------------------------------------------------

type Listener = () => void;
const listeners = new Map<StoreName, Set<Listener>>();

/** Subscribe to writes on the given stores; returns an unsubscribe function. */
export function subscribe(stores: StoreName[], fn: Listener): () => void {
  for (const s of stores) {
    let set = listeners.get(s);
    if (!set) {
      set = new Set();
      listeners.set(s, set);
    }
    set.add(fn);
  }
  return () => {
    for (const s of stores) {
      listeners.get(s)?.delete(fn);
    }
  };
}

function notify(name: StoreName): void {
  for (const fn of listeners.get(name) ?? []) {
    fn();
  }
}
