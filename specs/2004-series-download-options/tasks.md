# Tasks: Order series download options by season then size

**Feature**: `specs/2004-series-download-options` | **Branch**: `fix/2004-series-download-options`

- [x] T001 [US1] Add `src/services/quality-sort.ts` (pure `sizeMB`, `seasonNum`, `bySeasonThenSize`)
  with `src/services/quality-sort.test.ts` (TDD): series → season asc then size desc; movies → size
  desc; size parsing. Add the module to `vitest.config.ts` coverage include.
- [x] T002 [US1] In `src/components/SourceTitleModal.vue`, use `bySeasonThenSize` for `visibleQualities`,
  `firstUsableIn`, and the initial default sort; import `sizeMB`/`bySeasonThenSize` and drop the inline
  `sizeMB`.
- [x] T003 Gate: `npm run build` + `npm run test:unit:coverage`; `make roadmap`; bump version; open PR.
  Manual: a multi-season series lists Season 1,2,3… largest-first within each; movies unchanged.
