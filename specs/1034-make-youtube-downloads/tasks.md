# Tasks: Make YouTube downloads readable in the task list

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-07

## Phase 1: Learn the description (US1)

- **T001** `[F]` (FR-001, FR-002, FR-003) Failing tests for the lookup: a good
  response yields title + uploader + artwork; a timeout, a non-200, garbage
  JSON, and an oversized body each yield NOTHING and no error worth failing on —
  `server/internal/ytdl/meta_test.go`.
- **T002** `[F]` Implement the bounded lookup — `server/internal/ytdl/meta.go`.
- **T003** `[F]` (FR-006) Test + implement the name for a playlist or channel,
  which must describe the collection rather than a track within it —
  `server/internal/ytdl/meta*.go`.
- **T004** `[F]` Carry title, uploader and artwork as Job annotations —
  `server/internal/ytdl/job.go` + `job_test.go`.
- **T005** `[US1]` (FR-003) Handler test: submission succeeds and creates the Job
  even when the lookup fails outright —
  `server/internal/api/ytdl_handlers_test.go`.
- **T006** `[US1]` Look up on submit and expose the fields on the view —
  `server/internal/api/ytdl_handlers.go`.
- **T007** `[US1]` (FR-004, FR-005, FR-011) Render title and uploader, falling
  back to the readable link; long and non-Latin titles must not change the row's
  height — `src/services/api.ts`, `src/components/YtdlItem.vue`.
- **T008** `[US1]` e2e: a row names the item rather than showing its URL —
  `e2e/stateful/`.

**Checkpoint**: the list is readable. Artwork is still the placeholder.

## Phase 2: Artwork (US2)

- **T009** `[US2]` (FR-009) Failing tests for the proxy's host rule: this
  feature's artwork hosts are served; everything else is refused, including a
  host that the SOURCE image proxy would accept — the two lists must not be one
  — `server/internal/api/ytdl_thumb_test.go`.
- **T010** `[US2]` (FR-008) Implement the proxy — `server/internal/api/ytdl_thumb.go`,
  `router.go`.
- **T011** `[US2]` (FR-007) Render the artwork with a fallback to the existing
  icon on absence or load failure — `src/components/YtdlItem.vue`.
- **T012** `[US2]` e2e: a row shows artwork, and one without it still renders —
  `e2e/stateful/`.

## Phase 3: Gates

- **T013** `npm run build` · **T014** `npm run test:unit:coverage` ·
  **T015** `cd server && go build ./... && go vet ./... && go test ./...` ·
  **T016** `npm run test:e2e` · **T017** `make roadmap`, status → `in-review`.

## Dependencies

Phase 1 is independently shippable and delivers most of the value; Phase 2
depends only on the annotations from T004.
