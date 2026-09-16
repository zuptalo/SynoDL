/**
 * Builds a link to a title's page on IMDb (spec 1019).
 *
 * The id reaches us in two shapes depending on which provider call produced the
 * title: the listing endpoint returns a bare `tt2948372`, while the detail
 * endpoint returns a full `https://www.imdb.com/title/tt2948372` URL. Both
 * normalise to the same canonical link.
 *
 * The value is provider data being interpolated into an `href`, so this is
 * deliberately strict rather than forgiving: only `tt` followed by digits is
 * accepted, and anything else — a person id, a query string, a path traversal, a
 * lookalike host, a `javascript:` URL — yields "" so the caller renders plain
 * text instead of a link it can't vouch for.
 */

// A canonical IMDb title id: "tt" + digits, nothing else. Ids have grown from 7
// to 8+ digits over the years, so the length is deliberately unbounded.
const TITLE_ID_RE = /^tt\d+$/;

// A full IMDb title URL, from which we take only the id. The host must be imdb.com
// itself (optionally www.), so a lookalike like imdb.com.evil.example can't match.
const TITLE_URL_RE = /^https?:\/\/(?:www\.)?imdb\.com\/title\/(tt\d+)\/?$/;

/** The title's IMDb URL, or "" when the id is missing or not a title id. */
export function imdbUrl(imdbId: string): string {
  const raw = (imdbId ?? '').trim().toLowerCase();
  if (!raw) return '';

  const id = TITLE_ID_RE.test(raw) ? raw : (TITLE_URL_RE.exec(raw)?.[1] ?? '');
  return id ? `https://www.imdb.com/title/${id}/` : '';
}

// A canonical IMDb PERSON id: "nm" + digits (spec 0014). Bounded at both ends
// like the title id above, and for the same reason — this value is interpolated
// into an href, and a title id, a query string or a `javascript:` URL must not
// pass for a person.
const PERSON_ID_RE = /^nm\d{6,9}$/;

// A full IMDb person URL, from which we take only the id. The host must be
// imdb.com itself (optionally www.), so imdb.com.evil.example can't match.
const PERSON_URL_RE = /^https?:\/\/(?:www\.)?imdb\.com\/name\/(nm\d{6,9})\/?$/;

/** A person's IMDb URL, or "" when the id is missing or is not a person id. */
export function imdbPersonUrl(imdbId: string | undefined): string {
  const raw = (imdbId ?? '').trim().toLowerCase();
  if (!raw) return '';

  const id = PERSON_ID_RE.test(raw) ? raw : (PERSON_URL_RE.exec(raw)?.[1] ?? '');
  return id ? `https://www.imdb.com/name/${id}/` : '';
}
