import { describe, it, expect } from 'vitest';
import { initials } from './person';

describe('initials', () => {
  it('takes the first and last name', () => {
    expect(initials('Kurt Russell')).toBe('KR');
    expect(initials('Matthew McConaughey')).toBe('MM');
  });

  // A three-part name is first and last, not first and middle — "Mary Elizabeth
  // Winstead" reads as MW to anyone who knows her.
  it('skips the middle of a longer name', () => {
    expect(initials('Mary Elizabeth Winstead')).toBe('MW');
  });

  // A mononym gets one letter rather than a doubled one, which would read as a
  // stutter on the tile.
  it('gives a mononym a single letter', () => {
    expect(initials('Cher')).toBe('C');
  });

  // Names arrive as the source published them. One of the two sources publishes
  // in Persian, so this must not assume Latin — and must never transliterate.
  it('works on a non-Latin script', () => {
    expect(initials('پیتر داکتر')).toBe('پد');
  });

  it('ignores punctuation and extra whitespace', () => {
    expect(initials('  "Stone Cold"   Steve Austin ')).toBe('SA');
    expect(initials('Jean-Luc Picard')).toBe('JP');
  });

  // A name with no letters is not a name, and the tile shows a neutral glyph
  // rather than an empty circle.
  it('returns nothing for a name with no letters', () => {
    expect(initials('')).toBe('');
    expect(initials('   ')).toBe('');
    expect(initials('123 !!')).toBe('');
    expect(initials(undefined)).toBe('');
  });
});
