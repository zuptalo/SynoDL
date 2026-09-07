# Tasks: Give YouTube downloads their own button

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-07

## Phase 1: The dedicated sheet (US1) 🎯 MVP

- **T001** `[US1]` Failing e2e: the `+` menu offers a YouTube action; opening it
  shows a link field and a mode choice and NO destination, category, or file
  control — `e2e/stateful/ytdl-fab.spec.ts`.
- **T002** `[US1]` (FR-003, FR-004) Build the sheet: link field, Music /
  Music video segment with nothing preselected, Add disabled until a mode is
  chosen — `src/components/YoutubeDownloadModal.vue`.
- **T003** `[US1]` (FR-001, FR-002) Add the third FAB action, shown only when
  the server can run downloads, and mount the sheet —
  `src/views/tabs/TasksPage.vue`.
- **T004** `[US1]` (FR-007, FR-008) Report a rejected link in plain language,
  and report partial success when several links are submitted —
  `src/components/YoutubeDownloadModal.vue`.
- **T005** `[US1]` e2e: submitting a link from the new sheet puts a row in the
  Tasks list — `e2e/stateful/ytdl-fab.spec.ts`.

## Phase 2: Stop the old sheet misleading anyone (US2)

- **T006** `[US2]` (FR-005) Remove the Send-to segment and its submit branch —
  `src/components/NewTaskModal.vue`.
- **T007** `[US2]` (FR-006) Point at the dedicated button when a YouTube link is
  detected — `src/components/NewTaskModal.vue`.
- **T008** `[US2]` (FR-009) e2e: a YouTube link in the general sheet shows no
  mode selector, and a non-YouTube link behaves exactly as before —
  `e2e/stateful/ytdl-fab.spec.ts`.

## Phase 3: Gates

- **T009** `npm run build` · **T010** `npm run test:unit:coverage` ·
  **T011** `npm run test:e2e` · **T012** `make roadmap`, status → `in-review`.

## Dependencies

Phase 1 before Phase 2 — the replacement must exist before the old path is
removed, so no commit in between leaves the feature unreachable.
