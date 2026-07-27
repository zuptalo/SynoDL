// idb wrapper tests run against fake-indexeddb — a full in-memory IndexedDB —
// so the change bus and CRUD paths are exercised for real, no browser needed.
import 'fake-indexeddb/auto';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { IDBFactory } from 'fake-indexeddb';
import { clear, del, get, getAll, openDB, put, subscribe } from './idb';

beforeEach(() => {
  // A fresh IndexedDB per test; openDB's cached connection dies with it because
  // fake-indexeddb fires no events across factory swaps — reopen is implicit
  // since the cached promise belongs to the old factory only on first run.
  indexedDB = new IDBFactory();
});

describe('idb', () => {
  it('round-trips a record', async () => {
    await put('settings', { id: 'session', sid: 'abc', account: 'admin' });
    const row = await get<{ id: string; sid: string }>('settings', 'session');
    expect(row?.sid).toBe('abc');
  });

  it('returns undefined for a missing key and [] for an empty store', async () => {
    expect(await get('settings', 'nope')).toBeUndefined();
    expect(await getAll('favorites')).toEqual([]);
  });

  it('getAll returns every row', async () => {
    await put('history', { id: 'a', url: 'https://one' });
    await put('history', { id: 'b', url: 'https://two' });
    const rows = await getAll<{ id: string }>('history');
    expect(rows.map((r) => r.id).sort()).toEqual(['a', 'b']);
  });

  it('del removes a single row, clear removes all', async () => {
    await put('favorites', { id: 'x' });
    await put('favorites', { id: 'y' });
    await del('favorites', 'x');
    expect((await getAll('favorites')).length).toBe(1);
    await clear('favorites');
    expect(await getAll('favorites')).toEqual([]);
  });

  it('notifies subscribers of the affected store only', async () => {
    const onSettings = vi.fn();
    const onHistory = vi.fn();
    const unsub = subscribe(['settings'], onSettings);
    subscribe(['history'], onHistory);

    await put('settings', { id: 'k' });
    expect(onSettings).toHaveBeenCalledTimes(1);
    expect(onHistory).not.toHaveBeenCalled();

    await del('settings', 'k');
    expect(onSettings).toHaveBeenCalledTimes(2);

    unsub();
    await put('settings', { id: 'k2' });
    expect(onSettings).toHaveBeenCalledTimes(2); // unsubscribed — no more calls
  });

  it('upgrade creates all stores', async () => {
    const db = await openDB();
    expect(Array.from(db.objectStoreNames).sort()).toEqual(['favorites', 'history', 'settings']);
  });
});
