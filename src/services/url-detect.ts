/**
 * Extract downloadable URLs from free-form pasted text (spec 0001 FR-011).
 * Pure and on the vitest coverage gate. Only schemes Download Station accepts
 * are recognized — never javascript:, file:, or bare hostnames.
 */

// One token = scheme + at least one non-whitespace char. Tokenizing on
// whitespace first keeps magnet queries (which contain & and %XX) intact.
const URL_RE = /^(?:https?|ftps?):\/\/\S+$/i;
const MAGNET_RE = /^magnet:\?\S+$/i;
const THUNDER_RE = /^thunder:\/\/\S+$/i;

/** Free-form text → unique downloadable URLs, first-seen order. */
export function extractUrls(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const token of text.split(/\s+/)) {
    if (!token) continue;
    if (!URL_RE.test(token) && !MAGNET_RE.test(token) && !THUNDER_RE.test(token)) continue;
    if (seen.has(token)) continue;
    seen.add(token);
    out.push(token);
  }
  return out;
}
