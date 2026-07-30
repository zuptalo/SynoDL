# Tasks: Statistics filter segments sit in cards

**Feature**: `specs/2003-statistics-filter-segments` | **Branch**: `fix/2003-statistics-filter-segments`

- [ ] T001 [US1] In `src/components/StatisticsView.vue`, wrap the source segment (grouped with the
  admin user picker) and the history-range segment in `ion-item lines="none"` inside `ion-list inset`
  cards; add `.seg { width:100% }` and drop the floating `ion-segment { margin }`. Mirror the Settings
  "Open to" segment.
- [ ] T002 Gate: `npm run build` + `npm run test:unit`; `make roadmap`; bump version; open PR. Manual:
  segments now sit in cards, matching Settings; filtering/bucketing unchanged.
