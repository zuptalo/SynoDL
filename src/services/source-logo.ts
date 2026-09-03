/**
 * Which mark identifies a download source in the Discover grid.
 *
 * The marks are bundled rather than fetched from each source's own site, for
 * three reasons: they render instantly and offline in the PWA; they need no
 * addition to the outbound image allowlist; and — the deciding one — these sites
 * are periodically blocked, which is precisely when a user most needs to see
 * which source a result came from. A logo loaded from a blocked site would
 * vanish at exactly the wrong moment.
 *
 * A source with no bundled mark falls back to a monogram of its display name, so
 * adding a source never leaves a card unlabelled.
 */
import nama30 from '@/assets/sources/30nama.png';
import zarfilm from '@/assets/sources/zarfilm.png';

// Keyed by the driver's registry kind, which is stable — unlike the display
// name, which an operator chooses and can change at any time.
const MARKS: Record<string, string> = {
  zarfilm,
  '30nama': nama30,
};

/** The bundled mark for a source kind, or '' when there isn't one. */
export function logoForKind(kind: string | undefined): string {
  return MARKS[(kind ?? '').trim().toLowerCase()] ?? '';
}

/**
 * A short stand-in for a source with no bundled mark.
 *
 * Initials for a multi-word name ("My Movie Site" -> "MMS"), otherwise the
 * leading characters ("Filmstore" -> "FIL"). Capped at three so the chip stays
 * the size of a logo rather than becoming a second title.
 */
export function monogram(displayName: string | undefined): string {
  const name = (displayName ?? '').trim();
  if (!name) return '';
  const words = name.split(/[\s._-]+/).filter(Boolean);
  if (words.length > 1) {
    return words.slice(0, 3).map((w) => w[0]).join('').toUpperCase();
  }
  return name.slice(0, 3).toUpperCase();
}
