import { describe, expect, it } from 'vitest';
import { batch, extractUrls } from './url-detect';

describe('extractUrls', () => {
  it('extracts one URL per line', () => {
    expect(
      extractUrls('http://a.example/file.iso\nhttps://b.example/x.zip\nftp://c.example/y'),
    ).toEqual(['http://a.example/file.iso', 'https://b.example/x.zip', 'ftp://c.example/y']);
  });

  it('supports every downloadable scheme', () => {
    const input = [
      'http://h/f',
      'https://s/f',
      'ftp://f/f',
      'ftps://fs/f',
      'magnet:?xt=urn:btih:abcdef1234567890',
      'thunder://QUFodHRwOi8vZXhhbXBsZS5jb20vZi5pc29aWg==',
    ].join('\n');
    expect(extractUrls(input)).toHaveLength(6);
  });

  it('ignores junk between URLs and splits on any whitespace', () => {
    const input = 'check this: https://a.example/f.iso and also   ftp://b.example/g\nnot-a-url';
    expect(extractUrls(input)).toEqual(['https://a.example/f.iso', 'ftp://b.example/g']);
  });

  it('rejects lookalikes', () => {
    expect(extractUrls('http:/broken.example')).toEqual([]);
    expect(extractUrls('example.com/no-scheme')).toEqual([]);
    expect(extractUrls('javascript:alert(1)')).toEqual([]);
    expect(extractUrls('file:///etc/passwd')).toEqual([]);
  });

  it('dedupes while preserving first-seen order', () => {
    expect(extractUrls('https://a/1\nhttps://b/2\nhttps://a/1')).toEqual([
      'https://a/1',
      'https://b/2',
    ]);
  });

  it('empty and whitespace-only input give []', () => {
    expect(extractUrls('')).toEqual([]);
    expect(extractUrls('   \n\t ')).toEqual([]);
  });

  it('magnet links keep their query intact (no whitespace inside)', () => {
    const magnet = 'magnet:?xt=urn:btih:abc&dn=My%20File&tr=udp%3A%2F%2Ft.example%3A80';
    expect(extractUrls(`${magnet}\n`)).toEqual([magnet]);
  });

  it('splits bulk pastes on commas, semicolons, and tabs too (spec 1005)', () => {
    expect(extractUrls('https://a/1,https://b/2;https://c/3\thttps://d/4')).toEqual([
      'https://a/1',
      'https://b/2',
      'https://c/3',
      'https://d/4',
    ]);
    // Mixed and repeated delimiters collapse.
    expect(extractUrls('https://a/1 , ; https://b/2')).toEqual(['https://a/1', 'https://b/2']);
  });
});

describe('batch', () => {
  it('splits into fixed-size chunks (last chunk may be shorter)', () => {
    expect(batch([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
  });

  it('groups bulk URLs into batches of ten', () => {
    const urls = Array.from({ length: 23 }, (_, i) => `https://x/${i}`);
    const chunks = batch(urls, 10);
    expect(chunks.map((c) => c.length)).toEqual([10, 10, 3]);
  });

  it('an empty list yields no chunks', () => {
    expect(batch([], 10)).toEqual([]);
  });

  it('a non-positive size yields a single chunk (or none when empty)', () => {
    expect(batch([1, 2, 3], 0)).toEqual([[1, 2, 3]]);
    expect(batch([], 0)).toEqual([]);
  });
});
