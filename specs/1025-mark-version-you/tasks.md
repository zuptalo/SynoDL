# Tasks: Mark the version you actually downloaded

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Phase 1: Foundational — reading a release off a file name

- [x] T001 Failing tests for `ReleaseOf` in `server/internal/library/release_test.go`: reads resolution and group from the usual release-name shapes; normalises 4K/UHD onto 2160p; refuses a name carrying only one of the two; survives junk and empty names
- [x] T002 Implement `Release` and `ReleaseOf` in `server/internal/library/release.go`

## Phase 2: User Story 1 — mark only the version on disk (P1)

- [x] T003 [US1] Add `Owned bool` to `source.QualityOption` in `server/internal/source/source.go`, documented as handler-set so a driver cannot claim it
- [x] T004 [US1] Keep per-season release tokens on `evidenceRec` in `server/internal/api/library.go`, recorded from the file names `folderEvidence` already reads — season 0 carrying the title folder itself, so a movie uses the same field
- [x] T005 [US1] Implement option matching in `server/internal/api/library.go`: resolution AND group must agree, compared case-insensitively and ignoring separators; no match leaves every option unmarked
- [x] T006 [US1] Mark options on the way out of the title handler, leaving season presence and the downloading state untouched (FR-004, FR-005)
- [x] T007 [US1] Tests in `server/internal/api/library_test.go`: exactly one option marked for an identifiable release; nothing marked when the names identify nothing; two options differing only by group do not both match; a resolution-only agreement marks nothing; season presence is unaffected either way
- [x] T008 [US1] Seed the mock NAS fixtures with real-shaped release names in `server/internal/synomock/`, so the e2e can tell a matched option from an unmatched one

## Phase 3: User Story 2 — one season at a time (P1)

- [x] T009 [US2] Add `owned` to the client `QualityOption` in `src/services/api.ts`
- [x] T010 [US2] Group season-bearing options into an `ion-accordion-group` with `:multiple="false"` in `src/components/SourceTitleModal.vue` (FR-007, FR-009)
- [x] T011 [US2] Expand the first season not on the NAS on open, and none when every season is present (FR-008)
- [x] T012 [US2] Give each collapsed header its season, its on-the-NAS state and its option count (FR-010)
- [x] T013 [US2] Leave options with no season ungrouped (FR-011), and leave `selected` untouched by expansion (FR-012)

## Phase 4: User Story 3 — rows that fit (P2)

- [x] T014 [US3] Let the option row's title line wrap with the badge kept whole, and the episode list wrap rather than truncate, in `src/components/SourceTitleModal.vue` (FR-013)

## Phase 5: Cross-cutting

- [x] T015 e2e in `e2e/stateful/`: only the matching option is marked; the first missing season is the one expanded; expanding another collapses it; a fully-owned title opens collapsed
- [x] T016 e2e at a 360px viewport asserting no row is clipped (SC-005)
- [x] T017 Run the gates: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`
- [x] T018 Set the spec `**Status**:` to `in-review`, run `make roadmap`, commit, push, open the PR, merge, confirm k3s picks it up

## Dependencies

T002 → T004 → T005 → T006 → T007. T003 → T009. US1 must land before US2's header can say what is on the NAS per season, but the accordion itself (T010–T013) is independent of the matching.

## Added during implementation

- [x] T019 Extract the option row into `src/components/SourceQualityRow.vue` — the
  accordion needs the same row in two places (grouped and flat), and duplicating
  20 lines of markup would let the two drift
- [x] T020 Remove `markSeasonBreaks`/`GroupedOption` from `src/services/quality-sort.ts`
  and its tests: the season divider they drew is superseded by the accordion, and
  leaving them would be dead code
- [x] T021 Give the fake series three seasons in `server/internal/synomock/sources.go`
  — a one-season series cannot exercise "open the first season you do not have,
  and opening another closes it"
