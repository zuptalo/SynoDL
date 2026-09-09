/**
 * What Discover remembers between app launches (spec 1041).
 *
 * Results lived only in memory, so a cold start had nothing on screen and the
 * mount path read that as "nothing yet" and fetched — page one, plus however
 * many more it took to fill the grid. The source is shared and rate-limited, so
 * every launch spent requests on a view the reader may not have been there for.
 *
 * The rules for what may be restored are here, pure, because they are the part
 * that can be wrong in a way nobody notices: showing one source's catalog under
 * another's name, or restoring a page number that no longer matches the items it
 * came with, are both silent.
 */
import type { CatalogTitle, SourceSearchFilters } from '@/services/api';

/** The view that produced a set of results. */
export interface CatalogView {
  filters: SourceSearchFilters;
  sort: string;
  order: string;
  query: string;
  source: string;
  hideOwned: boolean;
}

/** One remembered session. `id` is the IndexedDB key; there is only ever one. */
export interface CatalogSession {
  id: 'last';
  view: CatalogView;
  items: CatalogTitle[];
  /** The last page fetched, and how many the source said there were. */
  page: number;
  pages: number;
  savedAt: number;
}

/**
 * How many titles are worth keeping.
 *
 * A bound, because this is written to a device on every search and an unbounded
 * scroll is not something to put there. Generous enough that an ordinary session
 * survives whole — past it, see below.
 */
export const MAX_REMEMBERED = 300;

/**
 * Build what to store from the state on screen.
 *
 * The awkward case is a session longer than the cap. Storing the first 300 items
 * while keeping the page number they came from would let infinite scroll fetch a
 * page whose items are ALREADY on screen — silent duplicates. So a truncated
 * session is stored as one with nothing more to load: it comes back as its first
 * stretch, and a pull-to-refresh is what gets the rest. Being honestly short
 * beats being quietly wrong.
 */
export function buildSession(
  view: CatalogView,
  items: CatalogTitle[],
  page: number,
  pages: number,
  now = Date.now(),
): CatalogSession {
  const base = { id: 'last' as const, view, savedAt: now };
  const session =
    items.length > MAX_REMEMBERED
      ? { ...base, items: items.slice(0, MAX_REMEMBERED), page: 1, pages: 1 }
      : { ...base, items: [...items], page, pages };
  return plain(session);
}

/**
 * A structured-cloneable copy.
 *
 * This is not defensive tidying — it is the whole reason storing worked at all.
 * What arrives here comes straight off Vue refs, so the objects are reactive
 * PROXIES, and IndexedDB refuses to clone a proxy. The write threw, the failure
 * was swallowed as "best effort", and every launch searched exactly as before
 * while the code looked correct. Nothing about it was visible until the request
 * count was counted.
 *
 * A JSON round-trip because that is precisely the shape being stored: catalog
 * metadata and a handful of scalars, no dates, no maps, nothing that survives
 * structured clone but not JSON.
 */
function plain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/**
 * Whether a remembered session may be shown.
 *
 * `available` is the set of source ids the instance currently offers, with ""
 * meaning "all sources". A session from a source that has since been removed or
 * disabled is DISCARDED rather than shown: putting one catalog on screen under
 * another's name is worse than spending a request.
 */
export function isRestorable(
  session: CatalogSession | undefined,
  available: string[],
): session is CatalogSession {
  if (!session || !Array.isArray(session.items) || session.items.length === 0) return false;
  if (!session.view || typeof session.view.sort !== 'string') return false;
  const src = session.view.source;
  if (src && !available.includes(src)) return false;
  return true;
}

/**
 * A stable fingerprint of a view, for asking "is what is on screen still what
 * the controls say?".
 *
 * Keys are sorted because the filters object is rebuilt from the server's JSON
 * on every load, and an incidental reordering there must not read as a change.
 */
export function viewFingerprint(v: CatalogView): string {
  const f = (v.filters ?? {}) as Record<string, unknown>;
  const entries = Object.keys(f)
    .sort()
    .map((k) => [k, f[k]]);
  return JSON.stringify([entries, v.sort, v.order, v.query, v.source, v.hideOwned]);
}
