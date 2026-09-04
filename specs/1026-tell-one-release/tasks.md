# Tasks: Tell one release from another by the file it makes

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Phase 1: Foundational — release identity

- [x] T001 Failing tests for `ReleaseKey` in `server/internal/library/release_test.go`, using the real names captured in the plan: the six qualities of one season all key differently; the same release at different episode numbers keys the same; case and separator differences do not matter; a name that identifies nothing keys empty
- [x] T002 Implement `ReleaseKey` and add `Key` to `Release` in `server/internal/library/release.go`, filled even when no resolution or group can be found

## Phase 2: User Story 1 — mark the option that made the file (P1)

- [x] T003 [US1] Add `ReleaseName string` with `json:"-"` to `source.QualityOption` in `server/internal/source/source.go`, documented as server-internal and unserialisable by construction (FR-002)
- [x] T004 [US1] Keep per-season release keys on `evidenceRec` in `server/internal/api/library.go`, from the file names already being read
- [x] T005 [US1] Make matching key-first in `server/internal/api/library.go`: an option naming a file is decided by the key alone — a mismatch is definitive and must NOT fall back to tokens (FR-004) — and an option naming nothing falls back to the existing comparison (FR-005)
- [x] T006 [US1] Tests in `server/internal/api/library_test.go`: the producing option is marked; two options a token cannot separate are told apart; a partial season still identifies its release; a file no option would produce marks nothing while the title stays present; a source naming no files behaves as before

## Phase 3: User Story 1 & 2 — ZarFilm supplies what it knows

- [x] T007 [US1] Supply `ReleaseName` from the download URL for ZarFilm movie options in `server/internal/source/providers/zarfilm.go`
- [x] T008 [US1] Supply `ReleaseName` from a season quality's first episode URL, leaving a paywalled row naming nothing
- [x] T009 [US2] Failing test then implementation for reading a season quality's encoder in `server/internal/source/providers/zarfilm_parse.go` — the segment after the final separator — leaving it empty when there is none (FR-008, FR-009)
- [x] T010 [US2] Expose that encoder on the season's `QualityOption` in `server/internal/source/providers/zarfilm.go`
- [x] T011 Name the fake source's files like real releases in `server/internal/synomock/sources.go`, including two that differ only by an encode a token cannot see

## Phase 4: Cross-cutting

- [x] T012 e2e in `e2e/stateful/`: two options at one resolution are told apart, and only the one on disk is marked
- [x] T013 Run the gates: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`
- [x] T014 Set the spec `**Status**:` to `in-review`, run `make roadmap`, commit, push, open the PR, merge, confirm k3s picks it up

## Dependencies

T002 → T004 → T005 → T006. T003 → T007/T008. T009 → T010. T011 → T012.

## Added during implementation

- [x] T015 Drop the encoder from a season option's LABEL now that it is shown in
  its own field, so the row does not print it twice
- [x] T016 Rewrite spec 1025's movie ownership e2e: that source now names the
  files its options produce, so the token path no longer applies to it — the test
  seeds the file an option actually makes, which also proves the same-resolution
  pair is separated for movies as well as series

## Note

`TestZarfilmPaywalledRowsNameNoFile` asserts on the movie path rather than the
series one: the fake site chooses its series branch before consulting the paywall
flag, so it cannot serve an unentitled series page at all. Worth knowing if a
future spec needs one — it would need a captured fixture that does not exist yet.
