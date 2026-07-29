import { describe, expect, it } from 'vitest';
import {
  countryOptions,
  genreOptions,
  languageOptions,
  qualityOptions,
  scoreOptions,
  typeOptions,
} from './facet-labels';

describe('facet-labels', () => {
  it('title-cases a genre slug, falling back to the name', () => {
    expect(genreOptions([{ value: '3373', slug: 'sci-fi', name: 'علمی-تخیلی' }])[0]).toEqual({
      value: '3373',
      label: 'Sci-Fi',
    });
    // No slug → the provider name.
    expect(genreOptions([{ value: '9', name: 'Magic' }])[0].label).toBe('Magic');
  });

  it('maps type codes (including the compound anime value) to English', () => {
    const opts = typeOptions([
      { value: '15' },
      { value: '16' },
      { value: '17&124913' },
      { value: '99999', name: 'Persian only' },
    ]);
    expect(opts.map((o) => o.label)).toEqual([
      'Movies',
      'Series',
      'Anime (series)',
      'Persian only', // unknown code → provider name
    ]);
  });

  it('formats score bands', () => {
    expect(scoreOptions([{ value: '9' }, { value: '8.5' }, { value: '-5' }]).map((o) => o.label)).toEqual([
      '9.0+',
      '8.5+',
      'Under 5.0',
    ]);
  });

  it('passes quality values through as labels', () => {
    expect(qualityOptions([{ value: '4K' }, { value: 'BluRay' }]).map((o) => o.label)).toEqual([
      '4K',
      'BluRay',
    ]);
  });

  it('localizes ISO country and language codes to English', () => {
    // Exact wording depends on the ICU build, so assert it localized (not the raw
    // code) rather than a fixed string.
    const us = countryOptions([{ value: 'US' }])[0].label;
    expect(us).not.toBe('US');
    expect(us.length).toBeGreaterThan(2);
    const en = languageOptions([{ value: 'en' }])[0].label;
    expect(en.toLowerCase()).toContain('english');
  });

  it('falls back to the provider name for an unlocalizable code', () => {
    // "XWG" (West Germany) isn't a current ISO region.
    expect(countryOptions([{ value: 'XWG', name: 'آلمان غربی' }])[0].label).toBe('آلمان غربی');
  });
});
