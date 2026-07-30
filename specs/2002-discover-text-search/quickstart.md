# Quickstart: verifying the Discover text-search fix

Prereqs: a running stack pointed at a real, configured 30nama source (the mock DSM/synomock does
not model 30nama). Use a deployed build or a backend with live source session material.

## 1. Type filter + search returns matching titles (US1 / FR-001, FR-002)

1. Open Discover.
2. Set the Type filter to **Movies**.
3. Search `batman`.
4. **Expect**: a non-empty grid of *movie* results (today: empty screen).
5. Change Type to **Series**, keep the query.
6. **Expect**: the grid changes to *series* results.
7. Clear the Type filter (Any type), keep the query.
8. **Expect**: mixed movie + series results.

## 2. Sort + advanced filters are honest during search (US2 / FR-004, FR-005)

1. With the search box empty (browse), confirm the sort control and genre/year/language/country/
   rating/quality filters are active and work.
2. Type a query.
3. **Expect**: the sort control and the non-type facet filters become visibly disabled with a short
   hint ("applies to browsing"); the Type filter stays enabled.
4. Clear the query.
5. **Expect**: sort + all filters re-enable and your previously chosen sort/filters are intact.

## 3. Browse unchanged (FR-006 / SC-003)

1. Empty query. Cycle sort through Most popular / IMDb / Release year / Recently added and toggle
   direction.
2. Apply a genre, a year range, a language.
3. **Expect**: results reorder / narrow exactly as before this change.

## Automated coverage

- `cd server && go test ./internal/source/...` — asserts `full_search` uses `type/all` and that the
  driver re-filters posts by `title_type` for a selected type (and does not for "all").
- `npm run test:unit` — asserts `useSourceCatalog` marks sort + non-type filters disabled when a
  query is present and re-enables them when it is cleared.
