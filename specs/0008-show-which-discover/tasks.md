# Tasks: Show which Discover titles you already have

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Branch**: `feat/0008-season-episode-ownership`

US1 shipped in 0.3.0 on a rule the spec licensed and practice disproved. These tasks
correct it and build US2/US3. TDD throughout (Principle II): the failing test comes first.

## Phase 1: Setup — make the mock able to disprove things

Blocking. Two bugs shipped in 0.3.0 because `synomock` accepted what DSM refuses; a flat
`uploads` map cannot express a season tree, so tests written against it would assert a
shape the NAS never returns.

- [x] T001 Replace the flat `uploads map[string][]string` with a per-directory file map mirroring `folders`, in `server/internal/synomock/synomock.go`
- [x] T002 Honour `filetype` (`dir` / `file` / `all`) in the FileStation `list` handler, returning files with `isdir:false`, in `server/internal/synomock/synomock.go`
- [x] T003 Extend `POST /__mock/library` to seed files per directory (the `tree` shape in `quickstart.md`), in `server/internal/synomock/synomock.go`
- [x] T004 Keep upload writing into that same file map so an uploaded file is listable afterwards, in `server/internal/synomock/synomock.go`

## Phase 2: Foundational — the pure pieces, and one client method

Blocks every user story. No I/O; all table-tested.

- [x] T005 [P] Failing tests for `IsVideo(name)` — video extensions true; `.nfo`, artwork and subtitle extensions false; case-insensitive; no extension false — in `server/internal/library/media_test.go`
- [x] T006 [P] Implement `IsVideo` over the extension table that already governs uploads, so one definition serves reading and writing, in `server/internal/library/media.go`
- [x] T007 [P] Failing tests for `EpisodeOf(name) (season, episode int, ok bool)` covering `S01E02`, `s1.e2`, `1x05`, underscore separators, and names with no match, in `server/internal/library/plexname_test.go`
- [x] T008 [P] Implement `EpisodeOf` by extending the existing `seasonEpisode` regex to return the episode number it already captures, in `server/internal/library/plexname.go`
- [x] T009 Failing test for `ListFiles(ctx, sid, path)` against `synomock`, asserting `filetype=file` reaches DSM and files come back, in `server/internal/syno/http_test.go`
- [x] T010 Implement `ListFiles` on `HTTPClient` and add it to the `syno.Client` interface — same allowlisted `SYNO.FileStation.List`, no discovery change — in `server/internal/syno/http.go` and `server/internal/syno/client.go`
- [x] T011 Add `ListFiles` to the fake client in `server/internal/api/fake_test.go`

## Phase 3: User Story 1 — a marker that means something (P1)

**Goal**: a title is marked present only when a video file is actually there, and a title
being downloaded says so instead.

**Independent test**: seed `Dune (2021)` holding a video and `Arrival (2016)` holding only
artwork; the first is marked, the second is not.

- [x] T012 [US1] Failing tests for `FolderEvidence` — video present, only sidecars, empty folder, unreadable listing — in `server/internal/api/library_test.go`
- [x] T013 [US1] Implement `folderEvidence(path)` returning `HasVideo`, cached per folder path with the 5-minute retention, in `server/internal/api/library.go`
- [x] T014 [US1] Failing test that a title matching NO folder triggers zero NAS listings (FR-010b), in `server/internal/api/library_test.go`
- [x] T015 [US1] Make the name index a candidate filter: verify only titles present in the current response, in `server/internal/api/library.go`
- [x] T016 [US1] Failing test that an unreadable folder yields `unknown`, never `absent` (FR-009, FR-010c), in `server/internal/api/library_test.go`
- [x] T017 [US1] Failing tests for `ownershipFor` — `downloading` outranks `owned` when an active task targets the folder (FR-001b) — in `server/internal/api/library_test.go`
- [x] T018 [US1] Derive `downloading` from the polled task list matched against the title folder, with no additional NAS read, in `server/internal/api/library.go`
- [x] T019 [US1] Replace `InLibrary bool` with `Ownership string` on `CatalogTitle` in `server/internal/source/source.go`
- [x] T020 [US1] Decorate search and title responses with `ownership`, keyed on title rather than id, in `server/internal/api/source_handlers.go`
- [x] T021 [P] [US1] Mirror `ownership` on the client `CatalogTitle` and drop `inLibrary`, in `src/services/api.ts`
- [x] T022 [US1] Render the owned marker only for `owned`, and nothing for `unknown`/`absent`, in `src/views/tabs/BrowserPage.vue`
- [x] T023 [US1] Add a distinct downloading marker, not colour-alone and announced to assistive tech (FR-011a, FR-012), in `src/views/tabs/BrowserPage.vue`
- [x] T024 [US1] e2e: seed a video-holding folder and a sidecar-only folder; assert exactly one card is marked (SC-001, SC-004a), in `e2e/stateful/discover-ownership.spec.ts`

## Phase 4: User Story 2 — which seasons, and which episodes (P2)

**Goal**: opening a series shows the seasons present and the episode numbers each holds.

**Independent test**: seed seasons 1–2 with a gap in season 2; both are marked and the gap
is visible.

- [x] T025 [P] [US2] Failing tests for `SeasonPresence` extraction — nested season folders, episodes flat in the title folder, unreadable numbering, a season holding only `season.nfo` — in `server/internal/api/library_test.go`
- [x] T026 [US2] Extend `folderEvidence` to collect seasons and episode numbers from one listing of the title folder plus one per season present, handling both nested season folders and episodes stored flat (FR-015), in `server/internal/api/library.go`
- [x] T027 [US2] Failing test that a season with `videoFiles > 0` and no readable numbering is still reported present (FR-016b), in `server/internal/api/library_test.go`
- [x] T028 [US2] Failing contract test for `GET /v1/library/title` — 200 for the degraded unknown case, 400 on bad `type`, no `total`/`complete` field ever emitted — in `server/internal/api/library_handlers_test.go`
- [x] T029 [US2] Implement `GET /v1/library/title` and route it, in `server/internal/api/library_handlers.go` and `server/internal/api/router.go`
- [x] T030 [US2] Rate-limit the lookup per user (FR-025b) and reject client-supplied paths (FR-025a), in `server/internal/api/library_handlers.go`
- [x] T030a [US2] Failing test that the lookup honours the caller's content-rating narrowing — a title the user cannot see in the catalog MUST NOT be answerable for (FR-025c) — in `server/internal/api/library_handlers_test.go`
- [x] T030b [US2] Apply the same rating filter the catalog applies before answering a lookup, so ownership is not a way around it (FR-025c), in `server/internal/api/library_handlers.go`
- [x] T031 [P] [US2] Add the client lookup returning seasons and episodes, in `src/services/api.ts`
- [x] T032 [US2] Mark present seasons and their episode numbers beside the download options, fetched in parallel so a failure only hides markers (FR-017); a movie shows as present with no season breakdown (FR-018), in `src/components/SourceTitleModal.vue`
- [x] T033 [US2] e2e: seed seasons 1–2 with a gap; assert the episodes shown and that no "complete" or "n of m" wording appears, in `e2e/stateful/discover-ownership.spec.ts`

## Phase 5: User Story 3 — guardrails (P3)

**Goal**: confirm before re-sending, and hide what the user already has or is fetching.

**Independent test**: sending a present title prompts; enabling the hide control removes
both owned and downloading titles.

- [x] T034 [US3] Failing tests that both `owned` and `downloading` trigger the confirmation (FR-019a), in `server/internal/api/source_handlers_test.go`
- [x] T035 [US3] Confirm before sending a title or season already present or downloading; cancelling sends nothing and consumes no allowance, accepting proceeds unchanged, and nothing absent ever prompts (FR-019, FR-020, FR-021), in `src/components/SourceTitleModal.vue`
- [x] T036 [US3] Add `hide_owned` to `source_prefs` as an appended migration, in `server/internal/store/`
- [x] T037 [US3] Persist and serve the toggle through the existing prefs endpoints so it follows the user across devices (FR-024), in `server/internal/api/source_prefs_handlers.go`
- [x] T038 [US3] Add the hide control to the filter sheet (FR-022), in `src/components/SourceFilterSheet.vue`
- [x] T039 [US3] Filter owned AND downloading where `comingSoon` is already filtered, keeping the backfill of FR-023a, in `src/composables/useSourceCatalog.ts`
- [x] T040 [US3] e2e: assert hiding removes both states and that the toggle survives a reload, in `e2e/stateful/discover-ownership.spec.ts`

## Phase 6: Polish

- [x] T041 [P] Confirm the Go coverage floor for `library` still holds and raise it if the new pure code exceeds it, in `.github/workflows/build-test.yml`
- [x] T042 [P] Verify no folder or file name reaches any log line, error payload or metric on either the success or failure path (FR-026, Principle III)
- [x] T042a Failing test that ownership never widens what a user may send — an owned title in a folder they lack permission for is still refused (FR-027) — in `server/internal/api/source_handlers_test.go`
- [x] T043 Measure NAS listings for a page of non-matching results and confirm it is zero (quickstart step 6)
- [x] T044 Run every gate: `npm run build`, `npm run test:unit:coverage`, `go build/vet/test`, `npm run test:e2e`
- [x] T045 Set `**Status**: shipped` in `spec.md` and run `make roadmap`

## Where the plan and the build diverged

Recorded rather than quietly reconciled, so the next reader is not misled by
tasks that describe something the code does differently.

- **T028–T030 (`GET /v1/library/title`)** — season detail rides on the EXISTING
  `GET /v1/source/title/{id}` instead of a new endpoint. That endpoint already
  resolves through the caller's own source access, so a user can only ask about a
  title they could already open — which satisfies FR-025a/FR-025c structurally,
  where a free-standing lookup keyed by title text would have needed its own rate
  limit and path guard to claw the same guarantees back. `contracts/library-api.md`
  was rewritten to match. T030a/T030b are covered by
  `TestTitleDetailAnswersOnlyForTitlesTheCallerHasSeen`.
- **T034 (confirmation tests)** — written against the client, not the server: the
  prompt is an `alertController` dialog and the send endpoint is unchanged, so
  there is no server behaviour to assert. FR-019/020/021 are exercised in
  `SourceTitleModal`.
- **T040 (hide-owned e2e)** — asserts a STORED preference reaching the grid and
  surviving a reload, rather than driving the filter sheet. The sheet is an Ionic
  modal whose Options section sits below the fold and keeps animating past
  Playwright's stability window; three attempts to click it were flaky and would
  have tested nothing the unit tests do not already cover, since the toggle only
  flips a boolean. The filtering itself is unit-tested in
  `useSourceCatalog.test.ts`.
- **T042 (log audit)** — found a real leak rather than confirming a clean bill: a
  failed upload logged the raw transport error, whose message embeds the request
  URL, and upload is the one call carrying `_sid` in the query string. Fixed and
  shipped in 0.6.2 with a regression test.

## Requirements unchanged by this amendment

FR-002, FR-003, FR-004, FR-006 and FR-013 govern how a catalog title is MATCHED to a folder
name — case, punctuation, articles, year forms, non-Latin scripts, and coexisting with the
"Soon" marker. All shipped in 0.3.0 and none change here: this amendment alters what a match
MEANS, not how one is found. Their existing tests must keep passing (T044) but no task
re-implements them.

## Dependencies

```
Phase 1 (mock)  ──►  Phase 2 (pure + client)  ──►  Phase 3 (US1)  ──►  Phase 4 (US2)
                                                          └────────►  Phase 5 (US3)
```

US2 and US3 both depend on US1's ownership state but not on each other. Phase 1 blocks
everything: without it the tests cannot express a season tree.

## Parallel opportunities

- T005–T008 are independent pure modules in separate files.
- T021, T031 are client-side and independent of the Go work once the contract is fixed.
- Within Phase 3, T022/T023 are one file and must be sequential.

## MVP

**Phase 1 + Phase 2 + Phase 3.** That alone corrects the shipped defect: no title is marked
present without a video file, and a downloading title says so. US2 and US3 add precision
and convenience on top.
