# Tasks: Fix Discover text search filtering

**Feature**: `specs/2002-discover-text-search` | **Branch**: `fix/2002-discover-text-search`

**Input**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[quickstart.md](./quickstart.md)

TDD is mandated (constitution): the test task in each story precedes its implementation task and MUST
fail first. No data-model / contract tasks — there are no new entities and the `/v1/source/search`
wire shape is unchanged.

## Phase 1: Setup

- [x] T001 Confirm the working tree is on `fix/2002-discover-text-search` and that
  `cd server && go test ./internal/source/...` and `npm run test:unit` both pass green as a baseline
  before any change.

## Phase 2: Foundational

_None._ The change is a targeted fix with no shared prerequisite; each story is independent.

## Phase 3: User Story 1 — Type filter + search returns matching titles (P1)

**Goal**: A text search with a Type filter set returns the matching titles of that type instead of an
empty screen.

**Independent test**: `go test` proves `full_search` uses `type/all` and that a selected type filter
drops non-matching `title_type` posts from the result.

- [x] T002 [US1] In `server/internal/source/providers/thirtynama_test.go`, update/extend
  `TestThirtynamaSearchQueryUsesFullSearch` (and add a focused case) to assert: (a) the `full_search`
  path is always `type/all` regardless of `Filters.Type`; (b) when `Filters.Type` is `movie`, posts
  whose `title_type` is not `movie` are dropped from `Search`'s returned items; (c) with no type
  filter, posts of all types are returned. Run and confirm the new assertions FAIL.
- [x] T003 [US1] In `server/internal/source/providers/thirtynama.go` `Search`, change the `full_search`
  branch to always build `full_search/type/all/orderby/relevant/order/desc/page/N` (drop the numeric
  `typeSeg`). After parsing the full_search title posts, if a type filter is set, keep only posts whose
  `title_type` matches the selected type — add a `titleTypeForCode` helper (15→movie, 16→series,
  17→anime) reusing/paralleling `typeCodes`. Leave the `advanced_search` (browse) branch untouched.
- [x] T004 [US1] Run `cd server && go test ./internal/source/... && go vet ./... && go build ./...`;
  confirm T002 assertions now pass and no other source test regressed.

**Checkpoint**: US1 complete — searching with a Type filter returns correctly-typed results.

## Phase 4: User Story 2 — Honest sort/filter controls during search (P2)

**Goal**: While a text query is active, the sort control and non-type facet filters are disabled with
a hint; clearing the query restores them without losing saved choices.

**Independent test**: `vitest` proves `useSourceCatalog` derives "controls disabled" when a query is
present and "controls enabled" when empty; the Browser view / filter sheet reflect it.

- [x] T005 [US2] In `src/composables/useSourceCatalog.ts`, add a derived `searchActive` computed
  (true when `query.value` is a non-empty trimmed string) and export it. Add a unit test in the
  composable's spec asserting `searchActive` flips with the query and does not mutate `sort`/`filters`
  (so cleared-query browsing restores the saved view). Run and confirm the new test FAILS.
- [x] T006 [US2] In `src/views/tabs/BrowserPage.vue`, disable the sort control when `searchActive` is
  true and show a short inline hint ("Sorting applies to browsing"). Keep the search box and Type
  affordance usable.
- [x] T007 [US2] In `src/components/SourceFilterSheet.vue`, when `searchActive` is true, disable the
  non-type facet controls (genre, release year, language, country, rating, quality, advanced facets)
  with a brief hint that they apply to browsing; keep the **Type** control enabled.
- [x] T008 [US2] Run `npm run test:unit` and `npm run build` (vue-tsc typecheck); confirm T005 passes
  and the client typechecks/builds.

**Checkpoint**: US2 complete — controls no longer mislead during search.

## Phase 5: Polish & Cross-Cutting

- [x] T009 Flip the spec `**Status**:` to `in-review` and run `make roadmap`; confirm `ROADMAP.md`
  regenerates and the `Roadmap up to date` guard would pass (no manual edits).
- [x] T010 Run the full local gate: `npm run build`, `npm run test:unit:coverage`,
  `cd server && go build ./... && go vet ./... && go test ./...`. All green.
- [x] T011 Manually walk `quickstart.md` §1–§3 against a build pointed at the real source (deployed or
  live-session backend); confirm US1, US2, and browse-unchanged.

## Dependencies & parallelism

- T002 → T003 → T004 (US1, strict TDD order; single Go file, not parallel).
- T005 → {T006, T007} → T008 (US2; T006 and T007 touch different files → `[P]`-eligible after T005).
- US1 (Phase 3) and US2 (Phase 4) are independent and MAY be done in either order; US1 is the MVP.
- Polish (Phase 5) runs last.

## Implementation strategy

MVP = User Story 1 (the empty-results bug). Ship US1 alone if needed; US2 is a trust/clarity
increment layered on top. Both ride the same PR into `main` (each task's `Closes #N` listed in the PR).


## Note on file names

T002/T003 name `thirtynama_test.go` and `thirtynama.go`. The provider was later
renamed to `nama30` (the site is "30nama"), so those paths no longer exist under
those names — the work itself is in `server/internal/source/providers/nama30.go`.
Left as written rather than rewritten: the tasks record what was done at the time,
and quietly editing history to match today's names would make them less truthful,
not more.
