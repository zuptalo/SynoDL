import { describe, expect, it } from 'vitest';
import { bySeasonThenSize, seasonNum, sizeMB } from './quality-sort';
import type { QualityOption } from '@/services/api';

function opt(p: Partial<QualityOption>): QualityOption {
  return {
    id: p.id ?? 'x',
    label: p.label ?? '',
    size: p.size ?? '',
    resolution: p.resolution ?? '',
    encoder: p.encoder ?? '',
    hardsub: p.hardsub ?? false,
    season: p.season,
    episodes: p.episodes,
  };
}

describe('sizeMB', () => {
  it('parses units to MB', () => {
    expect(sizeMB('1 TB')).toBe(1024 * 1024);
    expect(sizeMB('3 GB')).toBe(3072);
    expect(sizeMB('800 MB')).toBe(800);
    expect(sizeMB('2048 KB')).toBe(2);
    expect(sizeMB('100 KB')).toBe(0); // sub-MB rounds down to 0
    expect(sizeMB('')).toBe(0);
    expect(sizeMB('unknown')).toBe(0);
  });
});

describe('seasonNum', () => {
  it('reads the season number from the label, 0 when absent', () => {
    expect(seasonNum({ season: 'Season 6' })).toBe(6);
    expect(seasonNum({ season: 'Season 12' })).toBe(12);
    expect(seasonNum({ season: undefined })).toBe(0);
    expect(seasonNum({ season: '' })).toBe(0);
  });
});

describe('bySeasonThenSize', () => {
  it('orders series packs by season ascending, then size descending within a season', () => {
    const packs = [
      opt({ id: 's6-3g', season: 'Season 6', size: '3 GB' }),
      opt({ id: 's7-3g', season: 'Season 7', size: '3 GB' }),
      opt({ id: 's1-2g', season: 'Season 1', size: '2 GB' }),
      opt({ id: 's1-4g', season: 'Season 1', size: '4 GB' }),
      opt({ id: 's2-2g', season: 'Season 2', size: '2 GB' }),
    ];
    const order = [...packs].sort(bySeasonThenSize).map((q) => q.id);
    expect(order).toEqual(['s1-4g', 's1-2g', 's2-2g', 's6-3g', 's7-3g']);
  });

  it('orders movies (no season) by size descending only', () => {
    const movies = [
      opt({ id: 'm-1g', size: '1 GB' }),
      opt({ id: 'm-5g', size: '5 GB' }),
      opt({ id: 'm-2g', size: '2 GB' }),
    ];
    const order = [...movies].sort(bySeasonThenSize).map((q) => q.id);
    expect(order).toEqual(['m-5g', 'm-2g', 'm-1g']);
  });
});
