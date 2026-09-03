import { describe, expect, it } from 'vitest';
import { logoForKind, monogram } from './source-logo';

describe('logoForKind', () => {
  it('resolves a bundled mark for each configured source', () => {
    expect(logoForKind('zarfilm')).toBeTruthy();
    expect(logoForKind('30nama')).toBeTruthy();
  });

  it('is keyed on the registry kind, not the display name', () => {
    // An operator renames a source freely; the kind is what stays stable.
    expect(logoForKind('ZarFilm')).toBe(logoForKind('zarfilm'));
    expect(logoForKind('  30nama ')).toBe(logoForKind('30nama'));
  });

  it('returns empty for a source with no bundled mark', () => {
    expect(logoForKind('somethingelse')).toBe('');
    expect(logoForKind(undefined)).toBe('');
    expect(logoForKind('')).toBe('');
  });
});

describe('monogram', () => {
  it('uses initials for a multi-word name', () => {
    expect(monogram('My Movie Site')).toBe('MMS');
    expect(monogram('Alpha Beta')).toBe('AB');
  });

  it('caps at three so the chip stays a mark, not a title', () => {
    expect(monogram('One Two Three Four Five')).toBe('OTT');
    expect(monogram('Filmstore')).toBe('FIL');
  });

  it('splits on punctuation as well as spaces', () => {
    expect(monogram('film-store')).toBe('FS');
    expect(monogram('a.b.c')).toBe('ABC');
  });

  it('is empty for a missing name', () => {
    expect(monogram('')).toBe('');
    expect(monogram(undefined)).toBe('');
  });
});
