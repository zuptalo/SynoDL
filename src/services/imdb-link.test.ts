import { describe, it, expect } from 'vitest';
import { imdbUrl } from './imdb-link';

describe('imdbUrl', () => {
  it('builds the canonical title URL from a bare id', () => {
    expect(imdbUrl('tt2948372')).toBe('https://www.imdb.com/title/tt2948372/');
    // Ids vary in length (7 to 8+ digits) and keep growing.
    expect(imdbUrl('tt0111161')).toBe('https://www.imdb.com/title/tt0111161/');
    expect(imdbUrl('tt36460241')).toBe('https://www.imdb.com/title/tt36460241/');
  });

  it('normalises a full IMDb URL to the same canonical form', () => {
    // The source's detail endpoint returns the id as a full URL, so both shapes
    // reach the client depending on which call produced the title.
    expect(imdbUrl('https://www.imdb.com/title/tt2948372')).toBe('https://www.imdb.com/title/tt2948372/');
    expect(imdbUrl('https://www.imdb.com/title/tt2948372/')).toBe('https://www.imdb.com/title/tt2948372/');
    expect(imdbUrl('http://imdb.com/title/tt2948372/')).toBe('https://www.imdb.com/title/tt2948372/');
  });

  it('tolerates surrounding whitespace and letter case', () => {
    expect(imdbUrl('  tt2948372  ')).toBe('https://www.imdb.com/title/tt2948372/');
    expect(imdbUrl('TT2948372')).toBe('https://www.imdb.com/title/tt2948372/');
  });

  it('returns empty for anything that is not an IMDb title id', () => {
    for (const bad of ['', '   ', 'nm0000123', '2948372', 'tt', 'ttabcdefg', 'tt123abc']) {
      expect(imdbUrl(bad)).toBe('');
    }
  });

  it('never lets a malformed value leak into the URL', () => {
    // Defence in depth: the id is interpolated into an href, so nothing that
    // could steer the destination may survive.
    for (const bad of [
      'tt123/../../evil',
      'tt123?x=1',
      'tt123#frag',
      'javascript:alert(1)',
      'https://evil.example/title/tt123/',
      'https://www.imdb.com.evil.example/title/tt123/',
    ]) {
      expect(imdbUrl(bad)).toBe('');
    }
  });

  it('handles a missing value without throwing', () => {
    expect(imdbUrl(undefined as unknown as string)).toBe('');
    expect(imdbUrl(null as unknown as string)).toBe('');
  });
});
