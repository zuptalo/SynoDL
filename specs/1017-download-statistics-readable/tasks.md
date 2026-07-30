# Tasks: Download statistics — readable history + totals

**Feature**: `specs/1017-download-statistics-readable` | **Branch**: `feat/1017-download-statistics-readable`

**Input**: [plan.md](./plan.md), [spec.md](./spec.md)

Client-only. The pure bucketing change is TDD-first (coverage-gated module); the chart is CSS verified
on the build. One GitHub issue per task.

## Phase 1: Setup
- [ ] T001 Baseline green on the branch: `npm run test:unit` + `npm run build`.

## Phase 2: User Story 2 — Meaningful "All" range (P1, foundational for the chart)
- [ ] T002 [US2] In `src/services/stats-buckets.test.ts`, replace the "all collapses to one point"
  expectation with adaptive cases: multi-year span → one bar per year; single-year/multi-month →
  per month; single-month → per day; empty → empty. Run first (fails).
- [ ] T003 [US2] In `src/services/stats-buckets.ts`, make `bucketize('all')` pick a granularity from
  the span (years≥2 → year; else months≥2 → month; else day) and delegate, keeping totals conserved.

## Phase 3: User Story 1 — Readable chart (P1)
- [ ] T004 [US1] Rewrite `src/components/DownloadsChart.vue` as a responsive CSS/flex bar chart:
  bounded-width bars (min + max width), 2px gap, rounded tops, baseline + faint max reference, direct
  value labels when few (native tooltip via title otherwise), x-labels thinned when many with
  horizontal scroll, single accent-green series (labels in ink), and the empty state. No
  `preserveAspectRatio` stretch; correct in light/dark.

## Phase 4: User Story 3 — Total for user/range (P2)
- [ ] T005 [US3] In `src/components/StatisticsView.vue`, show a prominent total (count + total size,
  size "—" when no completed bytes) for the current user/source near the History section, using the
  existing `overall` aggregate; ensure it updates with the selection.

## Phase 5: Polish
- [ ] T006 Gate: `npm run build`, `npm run test:unit:coverage` (stats-buckets floors hold). Flip Status
  to `in-review`; `make roadmap`; bump version; open PR.
- [ ] T007 Manual verify on the deployed build: readable bars per period; "All" shows yearly/monthly
  bars (not one block); a single period is one normal bar; total count + size visible and correct.

## Dependencies
- T002 → T003 (TDD). T004 and T005 are independent (different files) and may run in parallel after the
  buckets change. T006/T007 last.

## Implementation strategy
MVP = US1 + US2 (a chart you can read). US3 (total) layers on. One PR into `main`.
