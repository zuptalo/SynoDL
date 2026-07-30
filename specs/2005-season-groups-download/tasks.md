# Tasks: Obvious season dividers in the download options list

**Feature**: `specs/2005-season-groups-download` | **Branch**: `fix/2005-season-groups-download`

- [ ] T001 [US1] Add `markSeasonBreaks()` (+ `GroupedOption`) to `src/services/quality-sort.ts` with
  unit tests in `quality-sort.test.ts` (TDD): flags the first row of each new season, never the first
  row overall, never for movies, preserves the options.
- [ ] T002 [US1] In `src/components/SourceTitleModal.vue`, map `visibleQualities` through
  `markSeasonBreaks` and apply a `.season-break` class (accent top border + spacing) to flagged rows.
- [ ] T003 Gate: `npm run build` + `npm run test:unit:coverage`; `make roadmap`; bump version; open PR.
  Manual: multi-season list shows clear season dividers; movies show none.
