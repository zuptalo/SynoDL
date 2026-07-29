import { describe, expect, it } from 'vitest';
import { bucketize, totalCount, type DayCount } from './stats-buckets';

// A small contiguous series spanning two months / two years for bucketing.
const days: DayCount[] = [
  { date: '2025-12-30', count: 1 }, // Tue, ISO week 2026-W01 region
  { date: '2025-12-31', count: 2 },
  { date: '2026-01-01', count: 3 },
  { date: '2026-01-02', count: 0 },
  { date: '2026-02-15', count: 4 },
];

describe('bucketize', () => {
  it('passes day granularity through with one point per day', () => {
    const pts = bucketize(days, 'day');
    expect(pts).toHaveLength(5);
    expect(pts.map((p) => p.count)).toEqual([1, 2, 3, 0, 4]);
  });

  it('sums by month with local-time boundaries', () => {
    const pts = bucketize(days, 'month');
    const byKey = Object.fromEntries(pts.map((p) => [p.key, p.count]));
    expect(byKey['2025-12']).toBe(3); // 1 + 2
    expect(byKey['2026-01']).toBe(3); // 3 + 0
    expect(byKey['2026-02']).toBe(4);
  });

  it('sums by year', () => {
    const pts = bucketize(days, 'year');
    const byKey = Object.fromEntries(pts.map((p) => [p.key, p.count]));
    expect(byKey['2025']).toBe(3);
    expect(byKey['2026']).toBe(7);
  });

  it('collapses all-time into a single total point', () => {
    const pts = bucketize(days, 'all');
    expect(pts).toHaveLength(1);
    expect(pts[0].count).toBe(10);
    expect(pts[0].label).toBe('All time');
  });

  it('conserves the total across every granularity', () => {
    const total = totalCount(days);
    for (const b of ['day', 'week', 'month', 'year', 'all'] as const) {
      const sum = bucketize(days, b).reduce((s, p) => s + p.count, 0);
      expect(sum).toBe(total);
    }
  });

  it('returns an empty series for no data', () => {
    expect(bucketize([], 'month')).toEqual([]);
    expect(bucketize([], 'all')).toEqual([{ key: 'all', label: 'All time', count: 0 }]);
  });
});
