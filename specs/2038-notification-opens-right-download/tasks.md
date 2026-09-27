# Tasks: Tapping a YouTube notification opens that download

**Input**: [spec.md](./spec.md)

- [x] T001 Failing e2e: `?download=<group id>` opens the playlist sheet, `?download=<single id>` the download sheet — neither the NAS sheet
- [x] T002 Server: `kind: "download"` on YouTube notification payloads (FR-001), with a test
- [x] T003 Service worker + App: route by kind, for the OS tap and the in-app "View" (FR-002)
- [x] T004 Tasks page: `download` query opens the right sheet, fetching a group by id when absent (FR-003)
- [x] T005 Gate: `go test`, `npm run build`, e2e
- [ ] T006 Merge and set `**Status**: shipped`
