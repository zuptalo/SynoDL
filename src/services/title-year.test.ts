import { describe, expect, it } from 'vitest';
import { splitYear } from './title-year';

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
