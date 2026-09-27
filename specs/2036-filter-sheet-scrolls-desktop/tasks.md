# Tasks: The filter sheet scrolls with a mouse wheel on desktop

**Input**: [spec.md](./spec.md)

- [x] T001 Reproduce: measure the sheet at 1280×720 / 1440×900 / 1920×1080 / 1280×600 — the list's end is below the window and the wheel does not move it
- [x] T002 Failing regression in `e2e/filter-sort.spec.ts`: wheel-scroll (not `click()`, which scrolls programmatically) must bring the end of the list into view — confirmed failing on the old sheet
- [x] T003 `:expand-to-scroll="false"` on `TaskFilterSheet.vue` and `SourceFilterSheet.vue` (FR-001)
- [x] T004 Gate: `npm run build`, unit coverage, filter e2e
- [ ] T005 Merge and set `**Status**: shipped`
