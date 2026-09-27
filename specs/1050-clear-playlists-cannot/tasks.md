# Tasks: Clear the playlists that can never finish, and retry refused downloads sooner

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

- [x] T001 `ytdl.Permanent` / `ytdl.AllPermanent`; group view carries `unrecoverable` (FR-001), with unit tests
- [x] T002 Tasks ⋯ menu: `Clear failed for good (N)` with confirmation, dismissing each (FR-002); e2e proves a refused playlist is spared
- [x] T003 Retry wait default 30 s; docs updated (FR-003)
- [x] T004 Gate: `go test`, unit coverage, `npm run build`, e2e
- [x] T005 Merge; set the production ConfigMap to 30 s; confirm; `**Status**: shipped` + `make roadmap`
