/**
 * Client-side time bucketing for the download history graph (spec 0006). The
 * server returns per-DAY counts (finest useful grain); this module aggregates
 * those into week / month / year / all-time buckets so switching granularity
 * needs no refetch (and coarser boundaries follow the viewer's LOCAL time).
 *
 * Pure and dependency-free so it carries a unit-test coverage floor.
 */

export type Bucket = 'day' | 'week' | 'month' | 'year' | 'all';

/** A day count row as returned by the server (date is "YYYY-MM-DD", UTC day). */
export interface DayCount {
  date: string;
  count: number;
}

/** A bucketed point for the chart: a label, a machine key, and the total. */
export interface BucketPoint {
  key: string;
  label: string;
  count: number;
}

// Parse "YYYY-MM-DD" into a LOCAL-time Date at midnight (not UTC), so week/month
// boundaries match what the viewer sees on their calendar.
function parseLocalDay(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, (m ?? 1) - 1, d ?? 1);
}

function pad(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

// The Monday-based ISO week key ("YYYY-Www") and a human label for a date.
function isoWeek(date: Date): { key: string; label: string } {
  // Copy; shift to the Thursday of this week to get the ISO week-year right.
  const d = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const day = (d.getDay() + 6) % 7; // Mon=0 … Sun=6
  d.setDate(d.getDate() - day + 3); // Thursday
  const firstThursday = new Date(d.getFullYear(), 0, 4);
  const week =
    1 +
    Math.round(
      ((d.getTime() - firstThursday.getTime()) / 86400000 - 3 + ((firstThursday.getDay() + 6) % 7)) / 7,
    );
  return { key: `${d.getFullYear()}-W${pad(week)}`, label: `W${week} ${d.getFullYear()}` };
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// The bucket key + label a given day falls into, for each granularity.
function bucketOf(date: Date, bucket: Bucket): { key: string; label: string } {
  const y = date.getFullYear();
  switch (bucket) {
    case 'day':
      return { key: `${y}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`, label: `${MONTHS[date.getMonth()]} ${date.getDate()}` };
    case 'week':
      return isoWeek(date);
    case 'month':
      return { key: `${y}-${pad(date.getMonth() + 1)}`, label: `${MONTHS[date.getMonth()]} ${y}` };
    case 'year':
      return { key: String(y), label: String(y) };
    case 'all':
      return { key: 'all', label: 'All time' };
  }
}

/**
 * Aggregate per-day counts into the chosen bucket. The server already zero-fills
 * days, so day-granularity output stays contiguous; coarser buckets sum the days
 * that fall within them, in chronological order. "all" collapses to one point.
 */
export function bucketize(days: DayCount[], bucket: Bucket): BucketPoint[] {
  if (bucket === 'all') {
    const total = days.reduce((sum, d) => sum + d.count, 0);
    return [{ key: 'all', label: 'All time', count: total }];
  }
  const order: string[] = [];
  const byKey = new Map<string, BucketPoint>();
  for (const d of days) {
    const { key, label } = bucketOf(parseLocalDay(d.date), bucket);
    let point = byKey.get(key);
    if (!point) {
      point = { key, label, count: 0 };
      byKey.set(key, point);
      order.push(key);
    }
    point.count += d.count;
  }
  return order.map((k) => byKey.get(k)!);
}

/** Grand total across all days (independent of bucket). */
export function totalCount(days: DayCount[]): number {
  return days.reduce((sum, d) => sum + d.count, 0);
}
