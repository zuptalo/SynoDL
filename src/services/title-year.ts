/**
 * The download source embeds a title's release year at the END of its title
 * string — a single year for a movie ("Esther 1986") or a range for a series
 * ("Breaking Bad 2008 - 2013", or "Show 2019 -" while it's still running). This
 * splits that trailing year off so the UI can show a clean title plus a separate
 * year field, while the raw title (kept intact by the caller) is still what we
 * send to the NAS — so the created subfolder keeps the year exactly as before.
 */

// Trailing year or year range, optionally parenthesized:
//   " 1986"  " (2014)"  " 2008 - 2013"  " 2019 -"  " 2008–2013"
const YEAR_RE = /\s*\(?\b((?:19|20)\d{2})\b(?:\s*[-–]\s*((?:19|20)\d{2})?)?\)?\s*$/;

export interface TitleYear {
  /** The title with the trailing year/range removed. */
  title: string;
  /** "1986", "2008 – 2013", or "2019 –" (ongoing); "" when no year is present. */
  year: string;
}

export function splitYear(raw: string): TitleYear {
  const title = (raw ?? '').trim();
  const m = YEAR_RE.exec(title);
  if (!m) return { title, year: '' };

  const start = m[1];
  const hasDash = /[-–]/.test(m[0]);
  let year = start;
  if (hasDash) year = m[2] ? `${start} – ${m[2]}` : `${start} –`;

  const clean = title.slice(0, m.index).trim();
  // Guard against a title that is ONLY a year (don't blank it out).
  return { title: clean || title, year: clean ? year : '' };
}

// A name that already ends in "(1999)", so a title the catalog happens to give
// us in the right shape is not given a second pair of parentheses.
const ALREADY_PAREN = /\((?:19|20)\d{2}\)\s*$/;

/**
 * The folder name a new download gets: "Title (Year)".
 *
 * The twin of PlexName in server/internal/library/plexname.go — the server is
 * what actually names the folder, so this exists purely so the upload modal can
 * PREVIEW the real destination. Showing the raw title instead told the user the
 * file was going to "movie/Test Movie 2024" when it was really going to
 * "movie/Test Movie (2024)". Keep the two in step.
 *
 * A range covering a series' run collapses to its first year, which is what a
 * scraper keys a show on. A title with no year, or one that IS a year ("1917"),
 * is returned untouched rather than gaining empty parentheses.
 */
export function plexName(raw: string): string {
  const s = (raw ?? '').trim();
  if (s === '' || ALREADY_PAREN.test(s)) return s;
  const { title, year } = splitYear(s);
  // splitYear reports no year both when there is none and when the year is the
  // whole title; either way the name stands as it is.
  if (!year) return s;
  return `${title} (${year.slice(0, 4)})`;
}

/**
 * Whether a title carries the release year a media server needs to identify it.
 *
 * Plex (and every other scraper) matches a movie on "Title (Year)" — given a bare
 * "rayan-test" it has nothing to look up, and the item lands in the library
 * unmatched, with no artwork or metadata. So the year is not decoration: it is
 * the whole identifier. This gates the upload rather than silently creating a
 * folder the user will have to rename later.
 *
 * A name already in final shape ("Dune (2021)") passes. A title that is ONLY a
 * year ("1917") does NOT: it still needs its own release year to be findable.
 */
export function isPlexReady(raw: string): boolean {
  const s = (raw ?? '').trim();
  if (s === '') return false;
  if (ALREADY_PAREN.test(s)) return true;
  return splitYear(s).year !== '';
}
