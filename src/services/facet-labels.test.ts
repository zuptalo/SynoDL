import { describe, expect, it } from 'vitest';
import { sortOptions } from '@/services/facet-labels';

describe('sortOptions', () => {
  it('keeps the order the server listed, rather than alphabetising', () => {
    const got = sortOptions([
      { value: 'favorite', slug: 'favorite', name: 'Most popular' },
      { value: 'imdb', slug: 'imdb', name: 'IMDb rating' },
      { value: 'date', slug: 'date', name: 'Recently added' },
    ]);
    // Orderings are a deliberate sequence (most-used first), unlike a country
    // list — re-sorting them would shuffle the control for no benefit.
    expect(got.map((o) => o.value)).toEqual(['favorite', 'imdb', 'date']);
    expect(got[0].label).toBe('Most popular');
  });

  it('falls back through slug then value for an ordering it has never seen', () => {
    // Hyphens are kept on purpose — the same title-caser labels genres, where
    // "sci-fi" must stay "Sci-Fi" rather than becoming "Sci Fi".
    expect(sortOptions([{ value: 'x', slug: 'recently-updated' }])[0].label).toBe('Recently-Updated');
    expect(sortOptions([{ value: 'mystery-order' }])[0].label).toBe('mystery-order');
  });

  it('returns nothing for no facets, so the caller can fall back to its built-in list', () => {
    expect(sortOptions([])).toEqual([]);
  });
});
