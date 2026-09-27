# Tasks: Removed videos are recognised however YouTube words it, and failed tracks lead a playlist

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

- [x] T001 `FailureFromOutput` matches every removal wording, with the real vKNZqM0d-xo output as a test case (FR-001)
- [x] T002 `sortPlaylistItems` in task-sort.ts (failed → active → waiting → finished, stable) used by the playlist sheet (FR-002), unit + e2e
- [x] T003 Gate: `go test`, unit coverage, `npm run build`, e2e
- [x] T004 Merge; retry the production track; confirm; `**Status**: shipped` + `make roadmap`
