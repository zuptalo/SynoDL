import { describe, it, expect, beforeEach, vi, type Mock } from 'vitest';
import { useSourceCatalog } from './useSourceCatalog';
import { api, ApiError, type CatalogTitle } from '@/services/api';

// Only the search call is faked — ApiError and everything else stay real, because
// the pagination guards branch on a genuine ApiError code.
vi.mock('@/services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api')>();
  return { ...actual, api: { ...actual.api, searchSource: vi.fn() } };
});
const searchSource = api.searchSource as unknown as Mock;

// searchActive drives the "disable sort + non-type filters during a text search"
// behaviour (spec 2002). It must be a pure read of the query and must never
// disturb the user's saved sort/filters, so clearing the query restores the same
// browse view.
describe('useSourceCatalog.searchActive', () => {
  const cat = useSourceCatalog();

  beforeEach(() => {
    cat.query.value = '';
  });

  it('is false for an empty or whitespace-only query', () => {
    cat.query.value = '';
    expect(cat.searchActive.value).toBe(false);
    cat.query.value = '   ';
    expect(cat.searchActive.value).toBe(false);
  });

  it('is true once a real query is entered', () => {
    cat.query.value = 'batman';
    expect(cat.searchActive.value).toBe(true);
  });

  it('does not mutate the chosen sort/order/filters (browse view survives a search)', () => {
    cat.sort.value = 'imdb';
    cat.order.value = 'asc';
    cat.filters.value = { genre: ['3362'] };

    cat.query.value = 'batman';
    expect(cat.searchActive.value).toBe(true);

    // The saved browse view is untouched, so clearing the query returns to it.
    expect(cat.sort.value).toBe('imdb');
    expect(cat.order.value).toBe('asc');
    expect(cat.filters.value).toEqual({ genre: ['3362'] });

    cat.query.value = '';
    expect(cat.searchActive.value).toBe(false);
  });
});

// searchIneffective flags when the user has selections the source ignores for text
// search (any non-type filter, or a non-default sort/order) — drives the chip/sort
// strike-through and the conditional hint nudge (spec 1014).
describe('useSourceCatalog.searchIneffective', () => {
  const cat = useSourceCatalog();

  beforeEach(() => {
    cat.query.value = '';
    cat.filters.value = {};
    cat.sort.value = 'favorite';
    cat.order.value = 'desc';
  });

  it('is false when not searching, even with ineffective selections', () => {
    cat.filters.value = { genre: ['3362'] };
    cat.sort.value = 'imdb';
    expect(cat.searchIneffective.value).toBe(false);
  });

  it('is false when searching with only a Type filter (type still applies)', () => {
    cat.filters.value = { type: 'movie' };
    cat.query.value = 'batman';
    expect(cat.searchIneffective.value).toBe(false);
  });

  it('is true when searching with a non-type filter', () => {
    cat.filters.value = { type: 'movie', genre: ['3362'] };
    cat.query.value = 'batman';
    expect(cat.searchIneffective.value).toBe(true);
  });

  it('is true when searching with a non-default sort', () => {
    cat.sort.value = 'imdb';
    cat.query.value = 'batman';
    expect(cat.searchIneffective.value).toBe(true);
  });

  it('is true when searching with a non-default order', () => {
    cat.order.value = 'asc';
    cat.query.value = 'batman';
    expect(cat.searchIneffective.value).toBe(true);
  });
});

// loadMore pulls a page BEYOND the one the user is about to reach, so a fast
// scroller doesn't sit watching the spinner at the bottom of every page
// (spec 1018). The load-ahead must still respect every existing stop condition.
describe('useSourceCatalog.loadMore', () => {
  const cat = useSourceCatalog();

  // A full page of results: enough that runSearch's viewport-fill loop (which
  // only tops up a thin FIRST page) never adds requests of its own here.
  const page = (n: number): CatalogTitle[] =>
    Array.from({ length: 24 }, (_, i) => ({
      id: `${n}-${i}`,
      type: 'movie',
      title: `Title ${n}-${i}`,
      posterUrl: '',
      imdbId: '',
      imdbScore: 0,
      providerScore: 0,
      plot: '',
      genres: [],
      comingSoon: false,
      freeDownload: false,
    }));

  const requestedPages = () => searchSource.mock.calls.map((c) => c[2]);

  beforeEach(() => {
    searchSource.mockReset();
    cat.query.value = '';
    cat.items.value = page(1);
    cat.page.value = 1;
    cat.pages.value = 10;
    cat.loading.value = false;
    cat.errorMsg.value = '';
    cat.needsRefresh.value = false;
    cat.unavailable.value = false;
    searchSource.mockImplementation(async (_q, _f, p: number) => ({ page: p, pages: 10, items: page(p) }));
  });

  it('loads two pages per trigger so the grid stays ahead of the scroll', async () => {
    await cat.loadMore();

    expect(requestedPages()).toEqual([2, 3]);
    expect(cat.page.value).toBe(3);
    // Each page appended as it arrived, rather than replacing what was shown.
    expect(cat.items.value).toHaveLength(72);
  });

  it('stops at the last page instead of requesting past the end', async () => {
    cat.pages.value = 2;
    searchSource.mockImplementation(async (_q, _f, p: number) => ({ page: p, pages: 2, items: page(p) }));

    await cat.loadMore();

    expect(requestedPages()).toEqual([2]);
    expect(cat.hasMore.value).toBe(false);
  });

  it('stops after the first page when the source starts failing mid-trigger', async () => {
    searchSource.mockRejectedValueOnce(new ApiError('source_needs_refresh', 409));

    await cat.loadMore();

    expect(requestedPages()).toEqual([2]);
    expect(cat.needsRefresh.value).toBe(true);
  });

  it('does nothing while a load is already in flight', async () => {
    cat.loading.value = true;

    await cat.loadMore();

    expect(searchSource).not.toHaveBeenCalled();
  });
});
