import { describe, expect, it } from 'vitest';
import {
  MAX_REMEMBERED,
  buildSession,
  isRestorable,
  viewFingerprint,
  type CatalogSession,
  type CatalogView,
} from './catalog-session';
import type { CatalogTitle } from './api';

const view = (over: Partial<CatalogView> = {}): CatalogView => ({
  filters: {},
  sort: 'date',
  order: 'desc',
  query: '',
  source: '',
  hideOwned: false,
  ...over,
});

const titles = (n: number): CatalogTitle[] =>
  Array.from({ length: n }, (_, i) => ({ id: String(i), title: `T${i}` }) as CatalogTitle);

describe('buildSession', () => {
  it('keeps an ordinary session whole, with the page it reached', () => {
    const s = buildSession(view(), titles(40), 3, 12);
    expect(s.items).toHaveLength(40);
    expect(s.page).toBe(3);
    expect(s.pages).toBe(12);
  });

  it('stores a very long session as one with nothing more to load', () => {
    // Keeping the page number alongside truncated items would let infinite
    // scroll fetch a page already on screen — silent duplicates. Being honestly
    // short beats being quietly wrong.
    const s = buildSession(view(), titles(MAX_REMEMBERED + 50), 9, 30);
    expect(s.items).toHaveLength(MAX_REMEMBERED);
    expect(s.page).toBe(1);
    expect(s.pages).toBe(1);
    expect(s.page < s.pages).toBe(false); // nothing more to append
  });

  it('copies the list rather than holding the live one', () => {
    const live = titles(3);
    const s = buildSession(view(), live, 1, 1);
    live.push(titles(1)[0]);
    expect(s.items).toHaveLength(3);
  });
});

describe('isRestorable', () => {
  const good: CatalogSession = {
    id: 'last',
    view: view({ source: '7' }),
    items: titles(5),
    page: 1,
    pages: 3,
    savedAt: 1,
  };

  it('accepts a session whose source is still offered', () => {
    expect(isRestorable(good, ['7', '9'])).toBe(true);
  });

  it('discards a session whose source has gone', () => {
    // Putting one catalog on screen under another's name is worse than spending
    // a request.
    expect(isRestorable(good, ['9'])).toBe(false);
    expect(isRestorable(good, [])).toBe(false);
  });

  it('accepts an all-sources session whatever is offered', () => {
    expect(isRestorable({ ...good, view: view({ source: '' }) }, [])).toBe(true);
  });

  it('refuses nothing, an empty result set, and a malformed record', () => {
    expect(isRestorable(undefined, ['7'])).toBe(false);
    expect(isRestorable({ ...good, items: [] }, ['7'])).toBe(false);
    expect(isRestorable({ ...good, items: undefined as never }, ['7'])).toBe(false);
    expect(isRestorable({ ...good, view: undefined as never }, ['7'])).toBe(false);
    expect(isRestorable({ ...good, view: { ...view(), sort: 1 as never } }, ['7'])).toBe(false);
  });
});

describe('viewFingerprint', () => {
  it('ignores the order the filter keys happen to be in', () => {
    // The filters object is rebuilt from the server's JSON on every load, and an
    // incidental reordering must not read as the view having changed.
    const a = viewFingerprint(view({ filters: { genre: 'x', year: '2020' } as never }));
    const b = viewFingerprint(view({ filters: { year: '2020', genre: 'x' } as never }));
    expect(a).toBe(b);
  });

  it('changes when anything the reader can see changes', () => {
    const base = viewFingerprint(view());
    expect(viewFingerprint(view({ sort: 'name' }))).not.toBe(base);
    expect(viewFingerprint(view({ order: 'asc' }))).not.toBe(base);
    expect(viewFingerprint(view({ query: 'dune' }))).not.toBe(base);
    expect(viewFingerprint(view({ source: '7' }))).not.toBe(base);
    expect(viewFingerprint(view({ hideOwned: true }))).not.toBe(base);
    expect(viewFingerprint(view({ filters: { genre: 'x' } as never }))).not.toBe(base);
  });
});

describe('buildSession — storability', () => {
  it('produces something IndexedDB can actually store', () => {
    // The bug this exists for: what is handed in comes off Vue refs, so the
    // objects are reactive PROXIES — and IndexedDB refuses to clone a proxy. The
    // write threw, the failure was swallowed as best-effort, and every launch
    // searched exactly as before while the code looked correct.
    const reactiveish = new Proxy(
      { filters: { genre: 'x' }, sort: 'date', order: 'desc', query: '', source: '', hideOwned: false },
      {},
    ) as unknown as CatalogView;
    const proxiedItems = titles(2).map((t) => new Proxy(t, {}) as CatalogTitle);

    const s = buildSession(reactiveish, proxiedItems, 1, 2);
    expect(() => structuredClone(s)).not.toThrow();
    expect(s.items).toHaveLength(2);
    expect(s.view.sort).toBe('date');
  });
});
