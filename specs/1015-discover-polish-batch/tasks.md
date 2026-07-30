# Tasks: Discover polish batch

**Feature**: `specs/1015-discover-polish-batch` | **Branch**: `feat/1015-discover-polish-batch`

**Input**: [plan.md](./plan.md), [spec.md](./spec.md)

Batch of small Discover tweaks shipped together. Coarse tasks → one GitHub issue each.

## Phase 1: Setup
- [ ] T001 Confirm baseline green on `feat/1015-discover-polish-batch` (branch off latest main):
  `npm run test:unit` + `npm run build` and `cd server && go test ./...`.

## Phase 2: User Story 1 — Remove the live-streamable filter (P1)
- [ ] T002 [US1] Remove the "Streamable only" toggle + its refs (ref/watch/apply/clear) from
  `src/components/SourceFilterSheet.vue`, and the stream active-filter chip from
  `src/views/tabs/BrowserPage.vue`.
- [ ] T003 [US1] Drop the `stream` filter field from the client model
  (`src/services/api.ts` `SourceSearchFilters`) and its uses in
  `src/composables/useSourceCatalog.ts` (`hasFilters`, `searchIneffective`).
- [ ] T004 [US1] Drop the server plumbing: `SearchFilters.Stream` (`source.go`), the `stream`
  request field + mapping (`api/source_handlers.go`), and the `buildParams` `stream` branch
  (`thirtynama.go`); update `thirtynama_test.go` to drop the `stream` assertion.

## Phase 3: Ship
- [ ] T005 Gate: `npm run build`, `npm run test:unit:coverage`, `cd server && go build/vet/test`.
  Flip Status to `in-review`, `make roadmap`, bump version, open PR into `main`.
- [ ] T006 Manual verify on the deployed build: the filter sheet has no "Streamable only" toggle;
  browse/search otherwise unchanged.

## Dependencies
- T002–T004 are one logical removal (client UI → client model → server); do together. T005/T006 last.

## Implementation strategy
US1 is the whole batch today; further batch items append as US2+ and ship in the same PR.
