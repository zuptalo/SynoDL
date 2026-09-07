import { describe, expect, it } from 'vitest';
import { displayYear, isPlausibleYear } from './release-year';

const THIS_YEAR = new Date().getFullYear();

describe('isPlausibleYear', () => {
  it('accepts years films actually exist in', () => {
    expect(isPlausibleYear('1927')).toBe(true);
    expect(isPlausibleYear('1974')).toBe(true);
    expect(isPlausibleYear(String(THIS_YEAR))).toBe(true);
    // Announced-but-unreleased titles legitimately sit slightly ahead.
    expect(isPlausibleYear(String(THIS_YEAR + 1))).toBe(true);
  });

  it('rejects what one source is known to publish for a body of its titles', () => {
    // FR-006: a missing year is a gap; a wrong one is misinformation.
    expect(isPlausibleYear('0')).toBe(false);
    expect(isPlausibleYear('1')).toBe(false);
    expect(isPlausibleYear('1800')).toBe(false);
    expect(isPlausibleYear(String(THIS_YEAR + 25))).toBe(false);
    expect(isPlausibleYear('')).toBe(false);
    expect(isPlausibleYear('not a year')).toBe(false);
  });
});

describe('displayYear', () => {
  it('prefers the year the source published', () => {
    expect(displayYear('1974', 'Gold 1974')).toBe('1974');
  });

  it('falls back to the year at the end of the title, which is all 30nama has', () => {
    expect(displayYear('', 'Gold 1974')).toBe('1974');
    expect(displayYear(undefined, 'Breaking Bad 2008 - 2013')).toBe('2008 – 2013');
  });

  it('shows nothing when neither knows, rather than a placeholder', () => {
    // FR-001: absence is honest; a guess is not.
    expect(displayYear('', 'Some Film With No Year')).toBe('');
    expect(displayYear(undefined, '')).toBe('');
  });

  it('drops an implausible source year instead of trusting it', () => {
    expect(displayYear('1', 'Gold 1974')).toBe('1974');
    expect(displayYear('1', 'Gold')).toBe('');
  });

  it('keeps a series range intact', () => {
    expect(displayYear('', 'Show 2019 -')).toBe('2019 –');
  });

  it('does not care that the title still contains the year, since the card strips it', () => {
    // FR-011: the caption renders splitYear().title, so the year appears once.
    expect(displayYear('1974', 'Gold 1974')).toBe('1974');
  });
});
