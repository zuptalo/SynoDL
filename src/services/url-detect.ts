/**
 * Extract downloadable URLs from free-form pasted text (spec 0001 FR-011,
 * bulk-paste delimiters spec 1005). Pure and on the vitest coverage gate. Only
 * schemes Download Station accepts are recognized — never javascript:, file:,
 * or bare hostnames.
 */

// One token = scheme + at least one non-whitespace char. Magnet queries use &
// and %XX (never , or ;), so splitting on commas/semicolons is safe for them.
const URL_RE = /^(?:https?|ftps?):\/\/\S+$/i;
const MAGNET_RE = /^magnet:\?\S+$/i;
const THUNDER_RE = /^thunder:\/\/\S+$/i;

// Bulk pastes separate links with any mix of whitespace (spaces, tabs, line
// breaks), commas, or semicolons.
const DELIMITERS = /[\s,;]+/;

/** Free-form text → unique downloadable URLs, first-seen order. */
export function extractUrls(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const token of text.split(DELIMITERS)) {
    if (!token) continue;
    if (!URL_RE.test(token) && !MAGNET_RE.test(token) && !THUNDER_RE.test(token)) continue;
    if (seen.has(token)) continue;
    seen.add(token);
    out.push(token);
  }
  return out;
}

/**
 * Split items into fixed-size chunks. Download Station caps how many URLs a
 * single create request accepts, so callers send bulk pastes in batches
 * (spec 1005). A non-positive size yields a single chunk.
 */
export function batch<T>(items: T[], size: number): T[][] {
  if (size < 1) return items.length ? [items] : [];
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) out.push(items.slice(i, i + size));
  return out;
}
