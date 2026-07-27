/**
 * Pure presentation formatters for sizes, speeds, progress, and time. These are
 * on the vitest coverage gate — keep them dependency-free and unit-tested.
 */

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** 1536 → "1.5 KB". Binary steps (1024), one decimal below 100, none above. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—';
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const rounded = value >= 100 || unit === 0 ? Math.round(value).toString() : value.toFixed(1);
  return `${rounded} ${UNITS[unit]}`;
}

/** Bytes/second → "8.1 MB/s". Zero reads as a dash — an idle row, not "0 B/s" noise. */
export function formatSpeed(bytesPerSec: number): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '—';
  return `${formatBytes(bytesPerSec)}/s`;
}

/** Downloaded/size → whole-percent string, clamped to [0, 100]. */
export function formatPercent(downloaded: number, size: number): string {
  if (!Number.isFinite(downloaded) || !Number.isFinite(size) || size <= 0) return '0%';
  const pct = Math.min(100, Math.max(0, (downloaded / size) * 100));
  return `${Math.floor(pct)}%`;
}

/** Fraction in [0,1] for progress bars. */
export function progressOf(downloaded: number, size: number): number {
  if (!Number.isFinite(downloaded) || !Number.isFinite(size) || size <= 0) return 0;
  return Math.min(1, Math.max(0, downloaded / size));
}

/**
 * Remaining time at the current rate → "2h 14m", "3m 20s", "45s". Unknown
 * (no rate, already done, or garbage input) reads as a dash.
 */
export function formatEta(remainingBytes: number, bytesPerSec: number): string {
  if (
    !Number.isFinite(remainingBytes) ||
    !Number.isFinite(bytesPerSec) ||
    remainingBytes <= 0 ||
    bytesPerSec <= 0
  ) {
    return '—';
  }
  const total = Math.round(remainingBytes / bytesPerSec);
  if (total < 60) return `${total}s`;
  if (total < 3600) return `${Math.floor(total / 60)}m ${total % 60}s`;
  if (total < 86400) return `${Math.floor(total / 3600)}h ${Math.floor((total % 3600) / 60)}m`;
  return `${Math.floor(total / 86400)}d ${Math.floor((total % 86400) / 3600)}h`;
}

/** Unix seconds → a compact local date-time like "26 Jul 14:03". */
export function formatDate(unixSeconds: number): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return '—';
  const d = new Date(unixSeconds * 1000);
  const day = d.getDate();
  const month = d.toLocaleString(undefined, { month: 'short' });
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${day} ${month} ${hh}:${mm}`;
}
