import { describe, expect, it } from 'vitest';
import { isPlexReady, plexName, splitYear } from './title-year';

describe('splitYear', () => {
  it('splits a trailing movie year', () => {
    expect(splitYear('Esther 1986')).toEqual({ title: 'Esther', year: '1986' });
    expect(splitYear('Interstellar 2014')).toEqual({ title: 'Interstellar', year: '2014' });
  });

  it('handles a parenthesized year', () => {
    expect(splitYear('Mothers’ Instinct (2024)')).toEqual({
      title: 'Mothers’ Instinct',
      year: '2024',
    });
  });

  it('splits a series year range', () => {
    expect(splitYear('Breaking Bad 2008 - 2013')).toEqual({
      title: 'Breaking Bad',
      year: '2008 – 2013',
    });
    expect(splitYear('Show 2008–2013')).toEqual({ title: 'Show', year: '2008 – 2013' });
  });

  it('marks an ongoing series with a trailing dash', () => {
    expect(splitYear('Severance 2022 -')).toEqual({ title: 'Severance', year: '2022 –' });
  });

  it('leaves a title with no year untouched', () => {
    expect(splitYear('A Thousand Little Cuts')).toEqual({
      title: 'A Thousand Little Cuts',
      year: '',
    });
  });

  it('does not treat a mid-title number as the year', () => {
    // Only a trailing year is stripped.
    expect(splitYear('2001: A Space Odyssey 1968')).toEqual({
      title: '2001: A Space Odyssey',
      year: '1968',
    });
  });

  it('keeps a title that is only a year', () => {
    expect(splitYear('1917')).toEqual({ title: '1917', year: '' });
  });
});

describe('plexName', () => {
  it('moves a trailing year into parentheses', () => {
    expect(plexName('Despicable Me 4 2024')).toBe('Despicable Me 4 (2024)');
    expect(plexName('Esther 1986')).toBe('Esther (1986)');
  });

  it("collapses a series' run to its first year, which is what scrapers key on", () => {
    expect(plexName('Friends 1994 - 2004')).toBe('Friends (1994)');
    expect(plexName('Show 2019 -')).toBe('Show (2019)');
  });

  it('leaves a name that is already in shape alone', () => {
    expect(plexName('Dune (2021)')).toBe('Dune (2021)');
  });

  it('does not add empty parentheses to a title with no year', () => {
    expect(plexName('Somewhere in Time')).toBe('Somewhere in Time');
    expect(plexName('')).toBe('');
  });

  it('keeps a title that IS a year whole rather than emptying it', () => {
    expect(plexName('1917')).toBe('1917');
    expect(plexName('1992 2024')).toBe('1992 (2024)');
  });

  it('is idempotent, so re-previewing never compounds the name', () => {
    for (const raw of ['Despicable Me 4 2024', 'Friends 1994 - 2004', '1917', 'Dune (2021)']) {
      expect(plexName(plexName(raw))).toBe(plexName(raw));
    }
  });
});

describe('isPlexReady', () => {
  it('accepts a title carrying a year, in either shape', () => {
    expect(isPlexReady('Dune 2021')).toBe(true);
    expect(isPlexReady('Dune (2021)')).toBe(true);
    expect(isPlexReady('Friends 1994 - 2004')).toBe(true);
  });

  it('rejects a title a scraper could not match', () => {
    expect(isPlexReady('rayan-test')).toBe(false);
    expect(isPlexReady('Some Movie')).toBe(false);
    expect(isPlexReady('')).toBe(false);
    expect(isPlexReady('   ')).toBe(false);
  });

  it('rejects a bare year, which still needs its own release year', () => {
    expect(isPlexReady('1917')).toBe(false);
  });

  it('agrees with plexName: anything it accepts gets parenthesised', () => {
    for (const raw of ['Dune 2021', 'Friends 1994 - 2004', 'Arrival 2016']) {
      expect(isPlexReady(raw)).toBe(true);
      expect(plexName(raw)).toMatch(/\((?:19|20)\d{2}\)$/);
    }
  });
});
