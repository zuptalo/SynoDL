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
