import { describe, expect, it } from 'vitest';
import { formatBytes, formatDate, formatEta, formatPercent, formatSpeed, formatTimestamp, progressOf } from './format';

describe('formatBytes', () => {
  it('formats each magnitude', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(999)).toBe('999 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(1048576)).toBe('1.0 MB');
    expect(formatBytes(6114656256)).toBe('5.7 GB');
    expect(formatBytes(2 ** 42)).toBe('4.0 TB');
  });
  it('drops the decimal at three digits', () => {
    expect(formatBytes(150 * 1024)).toBe('150 KB');
  });
  it('rejects garbage', () => {
    expect(formatBytes(-1)).toBe('—');
    expect(formatBytes(NaN)).toBe('—');
    expect(formatBytes(Infinity)).toBe('—');
  });
});

describe('formatSpeed', () => {
  it('appends /s', () => {
    expect(formatSpeed(8_500_000)).toBe('8.1 MB/s');
  });
  it('idles as a dash', () => {
    expect(formatSpeed(0)).toBe('—');
    expect(formatSpeed(-5)).toBe('—');
  });
});

describe('formatPercent / progressOf', () => {
  it('computes whole percent', () => {
    expect(formatPercent(25, 100)).toBe('25%');
    expect(formatPercent(999, 1000)).toBe('99%'); // floors, never rounds to a false 100%
  });
  it('clamps', () => {
    expect(formatPercent(200, 100)).toBe('100%');
    expect(formatPercent(-5, 100)).toBe('0%');
    expect(progressOf(200, 100)).toBe(1);
    expect(progressOf(-5, 100)).toBe(0);
  });
  it('handles zero size', () => {
    expect(formatPercent(0, 0)).toBe('0%');
    expect(progressOf(5, 0)).toBe(0);
  });
  it('progressOf is a clean fraction', () => {
    expect(progressOf(25, 100)).toBe(0.25);
  });
});

describe('formatEta', () => {
  it('scales units', () => {
    expect(formatEta(30, 1)).toBe('30s');
    expect(formatEta(200, 1)).toBe('3m 20s');
    expect(formatEta(8040, 1)).toBe('2h 14m');
    expect(formatEta(90000, 1)).toBe('1d 1h');
  });
  it('divides by rate', () => {
    expect(formatEta(1000, 100)).toBe('10s');
  });
  it('unknown reads as a dash', () => {
    expect(formatEta(0, 100)).toBe('—');
    expect(formatEta(100, 0)).toBe('—');
    expect(formatEta(NaN, 1)).toBe('—');
  });
});

describe('formatDate', () => {
  it('renders a compact local stamp', () => {
    // 2026-07-26 00:55 local — assert the shape, not the timezone-dependent hour.
    const out = formatDate(1774486500);
    expect(out).toMatch(/^\d{1,2} \w+ \d{2}:\d{2}$/);
  });
  it('zero and garbage read as a dash', () => {
    expect(formatDate(0)).toBe('—');
    expect(formatDate(NaN)).toBe('—');
  });
});

describe('formatTimestamp', () => {
  // FR-031. The format is fixed by the product: year-month-day, 24-hour, to the
  // second, on every device. These tests exist because the obvious
  // implementation — pick a locale that renders this shape — would put a
  // product decision inside locale data nobody would think to check.
  it('renders year-month-day and 24-hour time to the second', () => {
    // 2026-11-21 14:32:07 local time.
    const t = new Date(2026, 10, 21, 14, 32, 7).getTime() / 1000;
    expect(formatTimestamp(t)).toBe('2026-11-21 14:32:07');
  });

  it('pads every field, so widths never jump between rows', () => {
    const t = new Date(2026, 0, 5, 9, 8, 3).getTime() / 1000;
    expect(formatTimestamp(t)).toBe('2026-01-05 09:08:03');
  });

  it('uses 24-hour time, so afternoon is never ambiguous', () => {
    const t = new Date(2026, 10, 21, 23, 59, 59).getTime() / 1000;
    expect(formatTimestamp(t)).toBe('2026-11-21 23:59:59');
    const noon = new Date(2026, 10, 21, 12, 0, 0).getTime() / 1000;
    expect(formatTimestamp(noon)).toContain(' 12:00:00');
    const midnight = new Date(2026, 10, 21, 0, 0, 0).getTime() / 1000;
    expect(formatTimestamp(midnight)).toContain(' 00:00:00');
  });

  it('shows an em dash for a timestamp that does not exist yet', () => {
    // FR-033: a download that has not finished has no final timestamp, and 0
    // would render as 1970.
    expect(formatTimestamp(undefined)).toBe('—');
    expect(formatTimestamp(null)).toBe('—');
    expect(formatTimestamp(0)).toBe('—');
    expect(formatTimestamp(Number.NaN)).toBe('—');
  });
});
