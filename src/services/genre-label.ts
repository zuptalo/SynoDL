/**
 * One English word for a genre, whichever source supplied it (spec 1032).
 *
 * The two sources name the same genre differently, and neither is wrong:
 *   - 30nama publishes the English SLUG on each title ("sci-fi").
 *   - ZarFilm publishes its own Persian display name ("درام"), and separately
 *     publishes the Persian→English pairing in its filter facets, because its
 *     own genre routes are English.
 *
 * So a card can read half in English and half in Persian depending on where the
 * title came from. This resolves both to the same English label using the facet
 * list the filter sheet has already fetched — no extra request, and no
 * hand-written translation table that would rot as the taxonomy changes.
 *
 * Nothing is ever dropped for being unmapped: an unknown genre is shown as the
 * source wrote it, because a word the user cannot read still beats a blank.
 */
import type { SourceFacet } from '@/services/api';

/** ASCII letters, digits and hyphens — the shape of a slug, not a display name. */
function isAsciiSlug(v: string): boolean {
  return /^[A-Za-z0-9-]+$/.test(v);
}

/**
 * Title-case a slug: "sci-fi" → "Sci-Fi".
 *
 * Applied ONLY to things shaped like slugs. A display name ("No Slug", "درام")
 * is returned untouched — lower-casing the tail of a real name would turn
 * "No Slug" into "No slug", and Persian has no case to change anyway.
 */
function titleCaseSlug(v: string): string {
  if (!isAsciiSlug(v)) return v;
  return v
    .split('-')
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1).toLowerCase() : w))
    .join('-');
}

/** The English slug a source's own facets pair with this name, if any. */
function slugFor(value: string, facets?: SourceFacet[]): string | undefined {
  if (!facets?.length) return undefined;
  const hit = facets.find((f) => f.name === value || f.value === value);
  return hit?.slug ? hit.slug : undefined;
}

/** An English label for one genre as a source wrote it. */
export function genreLabel(raw: string, facets?: SourceFacet[]): string {
  const value = (raw ?? '').trim();
  if (!value) return '';
  return titleCaseSlug(slugFor(value, facets) ?? value);
}

/**
 * English labels for a title's genres: blanks dropped, duplicates collapsed,
 * and capped.
 *
 * De-duplication is not tidiness — it is the join. The same genre reaching us
 * as "drama" from one source and "درام" from the other must read as one genre,
 * or a combined grid shows it twice.
 */
export function genreLabels(
  raws: string[] | undefined,
  facets?: SourceFacet[],
  limit = Number.POSITIVE_INFINITY,
): string[] {
  const out: string[] = [];
  for (const raw of raws ?? []) {
    const label = genreLabel(raw, facets);
    if (!label || out.includes(label)) continue;
    out.push(label);
    if (out.length >= limit) break;
  }
  return out;
}
