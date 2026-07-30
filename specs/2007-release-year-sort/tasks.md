# Tasks: Release-year sort is fast again, and Discover opens on Most popular

**Feature**: `specs/2007-release-year-sort` | **Branch**: `fix/2007-release-year-sort`

- [x] T001 [US1] Replace `TestThirtynamaYearSortBounds` in
  `server/internal/source/providers/thirtynama_test.go` with a regression test that FAILS on the
  current code (TDD): the year sort sends no `min_year`/`max_year`, while a user-set year filter is
  still sent unchanged under any sort.
- [x] T002 [US2] Add a test asserting an empty and an unrecognised sort both resolve to
  `orderby/favorite`, not `orderby/year`.
- [x] T003 [US1] [US2] In `server/internal/source/providers/thirtynama.go`, drop the
  `yearSortMin`/`yearSortMax` injection and revert `buildParams` to take only the filters; change
  `orderbyField`'s fallback to `favorite`, with a comment recording the measured reason (the year
  bounds cost 15-20s per uncached page).
- [x] T004 Gate: `cd server && go build ./... && go vet ./... && go test ./...`; `make roadmap`;
  mark spec 2006 superseded. Manual on device: release-year pages load in the same range as the
  other sorts; a saved sort choice still wins.
