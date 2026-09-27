# Tasks: The whole download list is shown, not the newest page of it

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

- [x] T001 Store: page-size clamp (over the ceiling → ceiling), ceilings 500; items endpoint honours `?limit` (FR-003), with tests
- [x] T002 Client: `ytdlAll()` follows every page for the Tasks list; the playlist sheet loads every page before showing and drops the infinite scroll (FR-001, FR-002)
- [x] T003 e2e: more than one page of playlists all listed; more than one page of tracks all in the sheet
- [x] T004 Gate: `go test`, `npm run build`, unit coverage, e2e
- [x] T005 Merge; confirm in production; set `**Status**: shipped` and run `make roadmap`
