/**
 * Which release year to show, and whether to show one at all (spec 1032).
 *
 * Two sources, two habits. ZarFilm publishes a year field on each card.
 * 30nama publishes none at all — its years sit at the end of the title string,
 * which `title-year` already splits off so the heading reads cleanly.
 *
 * And one source carries a body of titles with implausible years (spec 2006
 * documented the same problem when sorting by year). A missing year is a small
 * gap the user can live with; a wrong one is misinformation on a card they are
 * using to decide. So anything outside the range films actually exist in is
 * dropped rather than shown.
 */
import { splitYear } from '@/services/title-year';

// The first public film screening was 1895; nothing in these catalogs predates
// it. The upper bound allows announced-but-unreleased titles, which legitimately
// sit a year or two ahead, without accepting a source's garbage.
const EARLIEST = 1895;
const LOOKAHEAD = 2;

/** The first 4-digit year in a value ("2008 – 2013" → 2008), or null. */
function firstYear(value: string): number | null {
  const m = /\b(\d{4})\b/.exec(value ?? '');
  return m ? Number(m[1]) : null;
}

/** Whether a year (or the start of a range) is one a film could have. */
export function isPlausibleYear(value: string): boolean {
  const y = firstYear(value ?? '');
  if (y === null) return false;
  return y >= EARLIEST && y <= new Date().getFullYear() + LOOKAHEAD;
}

/**
 * The year to display: the source's own field when it is believable, otherwise
 * the one at the end of the title, otherwise nothing.
 *
 * Returning "" is a real answer, not a failure — the caller renders no year
 * rather than a placeholder, because inventing one would be worse than omitting
 * it.
 */
export function displayYear(sourceYear: string | undefined, title: string): string {
  const published = (sourceYear ?? '').trim();
  if (published && isPlausibleYear(published)) return published;

  // The heading already has this stripped off by splitYear, so showing it here
  // separately puts the year on the card exactly once.
  const derived = splitYear(title ?? '').year;
  return derived && isPlausibleYear(derived) ? derived : '';
}
