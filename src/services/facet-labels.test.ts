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

  it('drops countries that no longer exist, and sorts the rest', () => {
    // Defunct states ride along in the provider's list under codes that aren't
    // current ISO regions ("XWG" West Germany, "DDDE" East Germany, "YUCS"
    // Yugoslavia, "SUHH" the USSR) — they have no English name, so they go.
    const opts = countryOptions([
      { value: 'US' },
      { value: 'XWG', name: 'آلمان غربی' },
      { value: 'DDDE', name: 'آلمان شرقی' },
      { value: 'YUCS', name: 'یوگسلاوی' },
      { value: 'SUHH', name: 'شوروی' },
      { value: 'CA' },
    ]);
    expect(opts.map((o) => o.value)).toEqual(['CA', 'US']);
  });

  it('lists English first, then the other languages alphabetically', () => {
    const opts = languageOptions([{ value: 'ja' }, { value: 'ar' }, { value: 'en' }, { value: 'de' }]);
    expect(opts[0].value).toBe('en');
    const rest = opts.slice(1).map((o) => o.label);
    expect(rest).toEqual([...rest].sort((a, b) => a.localeCompare(b, 'en')));
  });
});
