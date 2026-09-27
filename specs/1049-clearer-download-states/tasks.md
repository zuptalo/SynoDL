# Tasks: Download states named plainly, and every failed download retried in one tap

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

No plan/research artifacts: a label table, one menu item over an existing
endpoint, and the wording in tests.

- [x] T001 Shared `ytdl-labels.ts` (typed `Record<YtdlState, string>`) used by the row chip and the detail sheet; progress line says "finished" (FR-001, FR-002), with a unit test and coverage ratchet
- [x] T002 `Retry failed (N)` in the Tasks ⋯ menu, N in tracks, retried per row with `allSettled` (FR-003, FR-004), with an e2e test
- [x] T003 Gate: unit coverage, `npm run build`, e2e (ytdl suites)
- [ ] T004 Merge; confirm in production
- [ ] T005 Set `**Status**: shipped` and run `make roadmap`
