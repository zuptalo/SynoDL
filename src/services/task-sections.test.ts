import { describe, expect, it } from 'vitest';
import { newestOf, orderSections } from './task-sections';

describe('orderSections', () => {
  it('puts the block holding the newest thing first', () => {
    expect(orderSections({ uploads: 100, ytdl: 300, downloads: 200 })).toEqual([
      'ytdl',
      'downloads',
      'uploads',
    ]);
  });

  it('sorts a block with no timestamp last, not first', () => {
    // An absent time is not a recent one. Treating it as 0 would give the same
    // answer here for the wrong reason, and the wrong answer against negatives.
    expect(orderSections({ ytdl: 50, downloads: undefined, uploads: 10 })).toEqual([
      'ytdl',
      'uploads',
      'downloads',
    ]);
  });

  it('falls back to the fixed order when everything is empty', () => {
    expect(orderSections({})).toEqual(['uploads', 'ytdl', 'downloads']);
  });

  it('breaks a genuine tie by the fixed order, so the list cannot flicker', () => {
    expect(orderSections({ uploads: 7, ytdl: 7, downloads: 7 })).toEqual([
      'uploads',
      'ytdl',
      'downloads',
    ]);
  });

  it('always returns all three blocks', () => {
    expect(orderSections({ ytdl: 1 }).sort()).toEqual(['downloads', 'uploads', 'ytdl']);
  });
});

describe('newestOf', () => {
  it('takes the largest timestamp', () => {
    expect(newestOf([{ t: 3 }, { t: 9 }, { t: 5 }], (i) => i.t)).toBe(9);
  });

  it('is undefined for an empty list', () => {
    expect(newestOf([], () => 1)).toBeUndefined();
  });

  it('ignores entries with no usable time', () => {
    // A NAS task whose create time DSM never reported comes through as 0, and
    // an unfinished YouTube download has no submitted time at all. Neither is
    // "the epoch", and neither should win or block a real timestamp.
    expect(newestOf([{ t: 0 }, { t: undefined }, { t: 4 }], (i) => i.t)).toBe(4);
    expect(newestOf([{ t: 0 }, { t: undefined }], (i) => i.t)).toBeUndefined();
    expect(newestOf([{ t: Number.NaN }, { t: 2 }], (i) => i.t)).toBe(2);
  });
});
