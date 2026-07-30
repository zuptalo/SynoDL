import { describe, expect, it } from 'vitest';
import {
  countryOptions,
  genreOptions,
  languageOptions,
  passthroughOptions,
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
    // Sorted by label; an unknown code still falls back to the provider name.
    expect(opts.map((o) => o.label)).toEqual([
      'Anime (series)',
      'Movies',
      'Persian only',
      'Series',
    ]);
  });

  it('sorts every name-based facet alphabetically', () => {
    expect(genreOptions([{ value: '1', slug: 'western' }, { value: '2', slug: 'action' }]).map((o) => o.label)).toEqual(
      ['Action', 'Western'],
    );
    expect(qualityOptions([{ value: 'WEB-DL' }, { value: '4K' }, { value: 'CAM' }]).map((o) => o.label)).toEqual([
      '4K',
      'CAM',
      'WEB-DL',
    ]);
    expect(passthroughOptions([{ value: 'Netflix' }, { value: 'AMC' }]).map((o) => o.label)).toEqual([
      'AMC',
      'Netflix',
    ]);
  });

  it('leaves score bands in scale order (they are not names)', () => {
    expect(scoreOptions([{ value: '9' }, { value: '8' }, { value: '5' }]).map((o) => o.label)).toEqual([
      '9.0+',
      '8.0+',
      '5.0+',
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

  it('names the states that no longer exist in English, keeping them listed', () => {
    // These five are the source's full set of non-ISO country codes (verified
    // against the live facet list). Each fronts a real catalog — West Germany
    // alone runs to 27 pages — so they stay; they just can't be resolved by Intl
    // and would otherwise show the provider's Persian name.
    const opts = countryOptions([
      { value: 'XWG', name: 'آلمان غربی' },
      { value: 'SUHH', name: 'اتحاد جماهیر شوروی' },
      { value: 'CSHH', name: 'چکسلواکی' },
      { value: 'XYU', name: 'یوگسلاوی' },
      { value: 'DDDE', name: 'آلمان شرقی' },
    ]);
    expect(opts).toEqual([
      { value: 'CSHH', label: 'Czechoslovakia' },
      { value: 'DDDE', label: 'East Germany' },
      { value: 'SUHH', label: 'Soviet Union' },
      { value: 'XWG', label: 'West Germany' },
      { value: 'XYU', label: 'Yugoslavia' },
    ]);
  });

  it('still falls back to the provider name for an unknown country code', () => {
    // A code we've neither mapped nor Intl can name must not vanish from the
    // picker — there may be titles behind it.
    expect(countryOptions([{ value: 'ZZZZ', name: 'جایی' }])[0].label).toBe('جایی');
  });

  it('lists English first, then the other languages alphabetically', () => {
    const opts = languageOptions([{ value: 'ja' }, { value: 'ar' }, { value: 'en' }, { value: 'de' }]);
    expect(opts[0].value).toBe('en');
    const rest = opts.slice(1).map((o) => o.label);
    expect(rest).toEqual([...rest].sort((a, b) => a.localeCompare(b, 'en')));
  });
});
