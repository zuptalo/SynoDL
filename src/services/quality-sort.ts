/**
 * Ordering + size parsing for a title's download options (spec 2004). Pure and
 * dependency-light so it carries a unit-test coverage floor: the series-pack sort
 * (season, then size) is easy to get subtly wrong, so it's tested in isolation.
 */
import type { QualityOption } from '@/services/api';

/** Parse a provider size string ("11 GB", "800 MB") into MB; 0 when unknown. */
export function sizeMB(size: string): number {
  const m = /([\d.]+)\s*(TB|GB|MB|KB)/i.exec(size);
  if (!m) return 0;
  const v = parseFloat(m[1]);
  const unit = m[2].toUpperCase();
  return Math.round(unit === 'TB' ? v * 1024 * 1024 : unit === 'GB' ? v * 1024 : unit === 'KB' ? v / 1024 : v);
}

/** The season number parsed from a pack's season label ("Season 6" → 6); 0 for a
 *  movie or an unlabeled pack, so those keep plain largest-first ordering. */
export function seasonNum(q: Pick<QualityOption, 'season'>): number {
  const m = /(\d+)/.exec(q.season ?? '');
  return m ? Number(m[1]) : 0;
}

/**
 * Comparator for a title's download options. Series packs are grouped by season
 * ascending, then largest file first within a season; a movie (no season) has
 * season 0 for every option and so collapses to plain largest-first.
 */
export function bySeasonThenSize(a: QualityOption, b: QualityOption): number {
  return seasonNum(a) - seasonNum(b) || sizeMB(b.size) - sizeMB(a.size);
}

