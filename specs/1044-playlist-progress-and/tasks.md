# Tasks: A playlist shows how much of it is saved, and sorting by status puts what is running on top

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED, written first.

No plan/research artifacts: one derived count added to an existing response;
everything else is presentation and a pure sort function.

- [x] T001 Diagnose: every unfinished playlist is `downloading` (50 in production, ≤2 running); status sort inverted for the default Descending order, in both lists
- [x] T002 `YtdlCounts.Active` + `counts.active` in the list view, with a store test (FR-001)
- [x] T003 Failing unit tests in `src/services/{ytdl,task}-sort.test.ts`: active first in the default order; a playlist with a running track ahead of a waiting one; no `active` from an older server — confirmed failing
- [x] T004 Status sort as activity (higher = more active) in `ytdl-sort.ts` and `task-sort.ts` (FR-004, FR-005)
- [x] T005 `YtdlItem.vue`: saved ÷ total bar for a playlist; "waiting its turn" when nothing in it is running (FR-002, FR-003)
- [x] T006 e2e in `e2e/stateful/ytdl-expand.spec.ts`: a playlist waiting behind full slots says so; once a track runs it says downloading, with its bar at the saved fraction
- [x] T007 Gate: `npm run build`, unit coverage, `go build/vet/test`, stateful ytdl e2e
- [ ] T008 Merge; confirm in production
- [ ] T009 Set `**Status**: shipped` and run `make roadmap`
