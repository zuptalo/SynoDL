# Tasks: Tasks ordered by what is happening, and playlists that say why they failed

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

No plan/research artifacts: a default value, a stored-row migration, and one
derived line built from data the store already holds.

- [x] T001 Default sort = status (activity, newest first within), tests for both lists (FR-001)
- [x] T002 Versioned `taskFilter` row; old default migrated once, a chosen sort kept (FR-002)
- [x] T003 `ytdl.SummarizeFailures` + `Store.YtdlFailureReasons`; applied to the group view and the notification body (FR-003, FR-004), with tests
- [x] T004 Gate: `go test`, unit coverage, `npm run build`, e2e
- [x] T005 Merge; confirm in production
- [x] T006 Set `**Status**: shipped` and run `make roadmap`
