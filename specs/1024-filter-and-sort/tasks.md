# Tasks: Filter and sort every source the same way

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Phase 1: Foundational — capability plumbing

- [x] T001 Add `Sorts []FacetOption` to `source.SearchParameters` in `server/internal/source/source.go`, documenting that sort is a declared capability like any other facet
- [x] T002 Fold `Sorts` into `IntersectParameters` in `server/internal/source/merge.go`, with a test that a sort only one source offers drops from the combined set
- [x] T003 Collapse `facetKey` in `server/internal/source/merge.go` into one normalised key space so a slug and an equivalent label agree, with tests covering slug/slug, slug/label and label/label joins
- [x] T004 Capture a ZarFilm archive fixture holding the filter panel and the genre routes in `server/internal/source/providers/testdata/zarfilm/archive_filters.html`

## Phase 2: User Story 1 — filter and sort ZarFilm (P1)

- [x] T005 [US1] Failing tests for `parseFilterPanel` in `server/internal/source/providers/zarfilm_parse_test.go`: reads the sort, score and genre groups with their values and labels; returns nothing rather than erroring on a page with no panel
- [x] T006 [US1] Implement `parseFilterPanel` in `server/internal/source/providers/zarfilm_parse.go`
- [x] T007 [US1] Failing tests for `parseGenreSlugs`: pairs each Persian genre label with the English slug from the archive's own genre routes
- [x] T008 [US1] Implement `parseGenreSlugs` in `server/internal/source/providers/zarfilm_parse.go`
- [x] T009 [US1] Implement `zarfilm.Parameters()` in `server/internal/source/providers/zarfilm.go`: genres (Persian value, English slug, readable label), score bands, sorts and the existing types, built from one archive fetch
- [x] T010 [US1] Rewrite ZarFilm's browse URL construction in `server/internal/source/providers/zarfilm.go` to use `sortby`, `imdb_rate` and `filter_genre` — fixing the `filter=` name — so genre, score and sort compose and survive pagination for movies and series (FR-002, FR-003, FR-004)
- [x] T011 [US1] Drop sort for a text search, which the site ignores, rather than offering something that does nothing (FR-012)
- [x] T012 [US1] Extend the fake film site in `server/internal/synomock/sources.go` to serve the filter panel and to honour `sortby`, `imdb_rate` and `filter_genre`, so the e2e can tell a working filter from an ignored one
- [x] T013 [US1] Driver tests in `server/internal/source/providers/zarfilm_test.go`: the declared capabilities, and each parameter reaching the site with filters composed

## Phase 3: User Stories 2 & 3 — combined and single-source facet sets (P2)

- [x] T014 [US2] Declare 30nama's supported sorts in `server/internal/source/providers/nama30.go`
- [x] T015 [US2] Add a per-source capability cache to `server/internal/api/source_multi.go` with a TTL, reusing the `Parameters()` call `gatherParameters` already makes
- [x] T016 [US2] Translate a shared facet value into each source's own value before fan-out in `server/internal/api/source_handlers.go` (FR-005, FR-006)
- [x] T017 [US2] Skip and report a source that has no equivalent for a chosen value, with a new degraded reason, rather than sending it an unfiltered query (FR-007)
- [x] T018 [P] [US3] Tests in `server/internal/api/source_handlers_test.go`: combined offers the common set, single offers the full set, translation reaches each fake driver with its own value, and one source's capability failure costs only its own facets (FR-011)

## Phase 4: Client

- [x] T019 Prefer live sorts over the hardcoded list in `src/services/source-filters.ts` and `src/components/SourceFilterSheet.vue`, keeping the hardcoded list as the not-yet-loaded fallback
- [x] T020 [P] Unit tests for the sort fallback/preference in `src/services/`

## Phase 5: Cross-cutting

- [x] T021 e2e in `e2e/stateful/`: a filter narrows results for the HTML-shaped source, combined mode offers the common facets, and selecting one source restores its full set
- [x] T022 Run the gates: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`
- [ ] T023 Set the spec `**Status**:` to `in-review`, run `make roadmap`, commit, push, open the PR, merge, confirm k3s picks it up

## Dependencies

T001→T002, T004→T005/T007, T006+T008→T009, T009→T015→T016→T017. US1 is independently shippable; US2/US3 depend on T001 and on ZarFilm declaring anything at all (T009).

## Added during implementation

- [x] T024 Teach the fake JSON source to publish facets (`advanced_search_parametres`) and to honour a genre filter, so combined mode is exercised rather than assumed
- [x] T025 Fix the fake JSON source reading its filters as a raw JSON body when the real driver sends them **form-encoded** inside a `parameters` field — it was accepting every filter and applying none, which made a working translation and a broken one look identical
- [x] T026 Give the fake film site a filter panel, genre routes and real honouring of `filter_genre` / `imdb_rate` / `sortby`
