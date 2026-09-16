/**
 * What to draw when there is no photograph of somebody (spec 0014).
 *
 * Both sources have people they have no picture of, and so does IMDb, so this is
 * a common state rather than an error one: a broken-image glyph or a collapsed
 * tile would make the ordinary case look like a bug (FR-031).
 *
 * Names arrive as the source published them — Latin on one source, sometimes
 * Persian on the other — so this works on any script and never transliterates.
 * A name with no letters at all yields "", and the tile falls back to a neutral
 * glyph rather than an empty circle.
 */
export function initials(name: string | undefined): string {
  // Each word reduced to its first actual LETTER, so punctuation a source
  // prefixes ("Stone Cold" Steve Austin) does not become an initial.
  const letters = (name ?? '')
    .trim()
    .split(/[\s\u200c]+/)
    .map((word) => word.match(/\p{L}/u)?.[0] ?? '')
    .filter(Boolean);
  if (letters.length === 0) return '';

  // First and last, which is right for "Kurt Russell" and for a three-part name
  // — and one letter for a mononym rather than a doubled one, which would read
  // as a stutter on the tile.
  const last = letters.length > 1 ? letters[letters.length - 1] : '';
  return (letters[0] + last).toUpperCase();
}
