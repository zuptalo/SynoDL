import { describe, it, expect, beforeEach } from 'vitest';
import { useSourceCatalog } from './useSourceCatalog';

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
