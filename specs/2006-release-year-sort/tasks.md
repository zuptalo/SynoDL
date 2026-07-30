# Tasks: Release-year sort no longer leads with year-less titles

**Feature**: `specs/2006-release-year-sort` | **Branch**: `fix/2006-release-year-sort`

- [x] T001 [US1] Extend the driver tests in
  `server/internal/source/providers/thirtynama_test.go` first (TDD): browsing with the year sort sends
  `min_year`/`max_year`, a user-set bound wins over the default, and every other sort sends the
  parameters it sends today.
- [x] T002 [US1] In `server/internal/source/providers/thirtynama.go`, pass the sort into
  `buildParams` and fill the unset year bounds when the resolved `orderby` is `year`, with a comment
  recording why (the source's year-less / implausible-year rows) and the live-verified numbers.
- [x] T003 Gate: `cd server && go build ./... && go vet ./... && go test ./...`; `make roadmap`; bump
  version. Manual on device: release-year descending opens on the newest titles, ascending on the
  1890s, and a user-set year range still applies.
