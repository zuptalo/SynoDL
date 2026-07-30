# Research: Discover text search behaviour (30nama live API)

All findings below were verified against the **live** 30nama API on 2026-07-30 by driving the
site's own authenticated API client in-browser (so calls carried a valid Cloudflare clearance and
the `c-*` auth headers) and inspecting the returned payloads.

## Decision 1 — `full_search` only accepts `type/all`

- **Decision**: In `thirtynama.Search`, the `full_search` path MUST use `type/all`. Do not send the
  numeric type code in the path.
- **Evidence** (query `batman`):
  | path | result |
  |---|---|
  | `full_search/type/all/orderby/relevant/order/desc/page/1` | `success:true`, 12 pages, movie+series |
  | `full_search/type/15/...` (movie code) | `success:true`, **0 posts** |
  | `full_search/type/movie/...` (slug) | `success:true`, **0 posts** |
- **Rationale**: The current code sends `type/{typeParam(Type)}` = `type/15` for Movies, which the
  provider silently answers with an empty set — the root cause of "search returns nothing when a
  type is selected".
- **Preserve the Type filter**: `full_search` results carry `title_type` (`"movie"`/`"series"`/
  `"anime"`). Re-filter the parsed posts to the selected type server-side so the Type filter keeps
  narrowing search results. Map the stored numeric type code back to its `title_type` string for the
  comparison (15→movie, 16→series, 17→anime).
- **Alternatives considered**:
  - *Send `type/all` and drop the type filter entirely for search* — rejected: loses a useful,
    working narrowing the user explicitly asked for (chosen UX: honor type).
  - *Find another type token full_search accepts* — none exists; the live site itself only ever
    calls `full_search` with `type/all`.

## Decision 2 — `full_search` cannot sort or facet-filter

- **Decision**: Do not attempt to pass sort or facet filters to text search. Make the client honest
  instead: while a query is active, disable the sort control and non-type facet filters.
- **Evidence**:
  - The site's dedicated text-search endpoint `search/page/N/order/{dir}/orderby/{field}` returns
    **identical** results for `orderby/imdb` vs `orderby/relevant` (both: 8.4, 7.9, 8, 7.4, 6.1…) —
    the `orderby` is ignored for text search.
  - The same endpoint ignores a `parameters` body (genre filter did not change page count).
- **Rationale**: The provider ranks text results by relevance only. Offering sort/facet controls
  that silently no-op misleads the user; disabling them with a hint is the honest, low-risk fix.
- **Alternatives considered**:
  - *Switch text search to the `search` endpoint to gain sort* — rejected: it does not actually sort
    (proven above), so it buys nothing and widens the change.

## Confirmed correct (no change) — browse via `advanced_search`

For completeness, the browse path (empty query) was verified fully correct and MUST NOT change:

- **Sort** `favorite` / `imdb` / `year` / `date` all reorder correctly; **order** `asc` **and**
  `desc` both honored (asc imdb surfaces no-score titles first).
- **Filters** all reduce results with our exact param formats: `genre` numeric code as string/array
  (slug does NOT work — needs the code), `type` 15/16/17, `language` ISO (`ja`), `country` ISO
  (`KR`), `score`, `quality` string (`WEB-DL`), `min_year`/`max_year`.
- Our hardcoded codes in `src/services/source-filters.ts` match the live facet values.

## Notes on stateless/credential safety

No new source endpoints are introduced — `full_search`, `advanced_search`, and
`advanced_search_parametres` are already in use and on the allowlist. The change adds no persisted
state and touches no credentials, so the Principle III boundary and the `/speckit-checklist` gate
are unaffected.
