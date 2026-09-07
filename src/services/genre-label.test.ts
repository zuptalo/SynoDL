import { describe, expect, it } from 'vitest';
import { genreLabel, genreLabels } from './genre-label';
import type { SourceFacet } from '@/services/api';

// The facet list the filter sheet already loads: the provider's own name paired
// with the English slug it uses in its own genre routes.
const FACETS: SourceFacet[] = [
  { value: 'درام', name: 'درام', slug: 'drama' },
  { value: 'علمی-تخیلی', name: 'علمی تخیلی', slug: 'sci-fi' },
  { value: 'x', name: 'No Slug', slug: '' },
];

describe('genreLabel', () => {
  it('title-cases an English slug, which is what 30nama already publishes', () => {
    expect(genreLabel('drama', FACETS)).toBe('Drama');
    expect(genreLabel('sci-fi', FACETS)).toBe('Sci-Fi');
    expect(genreLabel('film-noir', FACETS)).toBe('Film-Noir');
  });

  it('maps a Persian display name through the facet list, which is the whole point', () => {
    expect(genreLabel('درام', FACETS)).toBe('Drama');
    expect(genreLabel('علمی تخیلی', FACETS)).toBe('Sci-Fi');
  });

  it('falls back to the raw value when nothing maps it, rather than dropping it', () => {
    // FR-005: still readable words, never an empty cell.
    expect(genreLabel('کمدی', FACETS)).toBe('کمدی');
    expect(genreLabel('No Slug', FACETS)).toBe('No Slug');
  });

  it('works with no facets loaded yet, since browsing can outrun the filter fetch', () => {
    expect(genreLabel('drama')).toBe('Drama');
    expect(genreLabel('درام')).toBe('درام');
  });

  it('never leaks an internal-looking value to the user', () => {
    // A hyphenated slug must not reach the card with its hyphens as-is.
    expect(genreLabel('romantic-comedy')).toBe('Romantic-Comedy');
    expect(genreLabel('')).toBe('');
    expect(genreLabel('   ')).toBe('');
  });

  it('is case-insensitive about the slug it is handed', () => {
    expect(genreLabel('DRAMA')).toBe('Drama');
    expect(genreLabel('Sci-Fi')).toBe('Sci-Fi');
  });
});

describe('genreLabels', () => {
  it('maps a list and drops blanks', () => {
    expect(genreLabels(['drama', '', 'sci-fi'], FACETS)).toEqual(['Drama', 'Sci-Fi']);
  });

  it('caps the list, because the caption has a fixed budget (FR-008)', () => {
    expect(genreLabels(['drama', 'sci-fi', 'film-noir'], FACETS, 1)).toEqual(['Drama']);
    expect(genreLabels(['drama', 'sci-fi', 'film-noir'], FACETS, 2)).toEqual(['Drama', 'Sci-Fi']);
  });

  it('de-duplicates once two sources map onto the same English genre', () => {
    // The join is the point: the same genre from both sources must read as one.
    expect(genreLabels(['drama', 'درام'], FACETS)).toEqual(['Drama']);
  });

  it('survives an absent list', () => {
    expect(genreLabels(undefined, FACETS)).toEqual([]);
  });
});
