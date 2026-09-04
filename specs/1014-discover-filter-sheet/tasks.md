# Tasks: Discover filter sheet polish

**Feature**: `specs/1014-discover-filter-sheet` | **Branch**: `feat/1014-discover-filter-sheet`

**Input**: [plan.md](./plan.md), [spec.md](./spec.md)

Client-only, presentation change. One piece of logic (`searchIneffective`) is unit-tested TDD-first;
the rest is markup/CSS verified on the build. Coarse tasks (one per user story + a foundation + a
polish task) → one GitHub issue each.

## Phase 1: Setup

- [ ] T001 Confirm baseline green on `feat/1014-discover-filter-sheet`: `npm run test:unit` and
  `npm run build` pass before changes.

## Phase 2: Foundational

- [ ] T002 In `src/composables/useSourceCatalog.ts`, add and export a `searchIneffective` computed
  (true when `searchActive` AND any non-type filter is set OR sort/order differ from the defaults),
  and add a failing-first unit test in `src/composables/useSourceCatalog.test.ts` covering: false
  when only Type is set, true when a genre/quality/year is set during search, true for a non-default
  sort during search, false when the query is empty. This derivation feeds US1 (marking) and US2
  (conditional nudge).

## Phase 3: User Story 1 — Mark ineffective sort/filters during search (P1)

- [ ] T003 [US1] In `src/views/tabs/BrowserPage.vue`, add an `ineffective` visual treatment
  (line-through + reduced opacity, theme-aware) applied to every active-filter chip EXCEPT the Type
  chip while `searchActive`, and to the sort control's text (`ion-select::part(text)` + the order
  toggle) while `searchActive`. Clearing the search removes the treatment.

## Phase 4: User Story 2 — Clean, scrolling hint (P2)

- [ ] T004 [US2] In `src/views/tabs/BrowserPage.vue`, remove the hint `ion-toolbar` from
  `<ion-header>` and render the hint at the top of the scrolling results (above the chips) so it
  scrolls away. Reword it with no dash; show the base sentence always while searching and append the
  "clear the search to sort or filter" nudge only when `searchIneffective`.

## Phase 5: User Story 3 — Settings-like sheet (P3)

- [ ] T005 [US3] In `src/components/SourceFilterSheet.vue`, change the Type and Min-rating selects
  from `interface="popover"` to `interface="alert"` to match the other facet selects.
- [ ] T006 [US3] In `src/components/SourceFilterSheet.vue`, regroup the controls into titled sections
  using `ion-list inset` + `ion-list-header` (e.g. Basics / Origin / Advanced / Year & people /
  Options) styled like the Settings screen; keep the searchActive disabling + hint from spec 2002.

## Phase 6: Polish

- [ ] T007 Run the gate: `npm run build`, `npm run test:unit:coverage`; server unchanged so
  `cd server && go build ./... && go test ./...` as a sanity check. Flip spec Status to `in-review`
  and `make roadmap`.
- [ ] T008 Manual check on the deployed build: search a term with genre/quality/year/sort active →
  non-type chips + sort struck through, Type normal; hint scrolls with chips, no dash, nudge only
  when ineffective selections exist; Type/Min rating open centered dialogs; sheet reads as sections.

## Dependencies

- T002 → T003, T004 (both consume the derivation). T003/T004 (same file, sequence to avoid conflict).
- T005, T006 independent of the above (different file) → parallel-eligible.
- T007/T008 last.

## Implementation strategy

MVP = US1 (the at-a-glance marking). US2 and US3 layer on. All ride one PR into `main`.


## Reconciled 2026-09-04

The spec is marked shipped because the feature is implemented and released —
verified against the code, not against these boxes. The checkboxes were never
maintained during implementation and are left as written: ticking them now would
claim each task was completed as specified, which is more than was checked.
