---
description: "Task list for spec 0008 — Show which Discover titles you already have"
---

# Tasks: Show which Discover titles you already have

**Input**: Design documents from `/specs/0008-show-which-discover/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/library-api.md](./contracts/library-api.md),
[checklists/security.md](./checklists/security.md)

**Tests**: REQUIRED, not optional. Constitution Principle II mandates that tasks order failing
tests before implementation (Red → Green → Refactor), that new proxy/session/handler logic ships
unit tests, and that new user-facing behaviour extends the `e2e/` suite.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: `[US1]` marker · `[US2]` season detail · `[US3]` guardrails

## Path Conventions

Monorepo: Go server under `server/`, Vue PWA at the repository root under `src/`, Playwright
under `e2e/`. Paths below are repository-relative.

---

## Phase 1: Setup

**Purpose**: Create the one new package and wire its quality gate before any logic exists.

- [ ] T001 Run `make roadmap` to regenerate `ROADMAP.md` with this spec's row and commit it. **Do this first**: CI's `Roadmap up to date` guard is always-on and fails on every push while `ROADMAP.md` lacks a row for 0008, so leaving it to the end leaves the branch red throughout
- [ ] T002 Create the new pure package with a doc comment explaining why matching lives outside `api/` (no I/O, exhaustively table-tested, carries a coverage floor) in `server/internal/library/library.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The matching rules and the library snapshot. Every user story reads from these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Tests (write first, watch them fail)

- [ ] T003 [P] Failing table tests for `Key()` in `server/internal/library/match_test.go` — every trailing-year form enumerated by `src/services/title-year.ts` (`Esther 1986`, `(2014)`, `2008 - 2013`, `2008–2013`, `2019 -`), leading articles, bracketed extras, punctuation and case differences, a title that is *only* a year, and **Persian titles, which must not reduce to the empty string** (an ASCII filter would collapse them all together)
- [ ] T004 [P] Failing tests for the year-agreement rule in `server/internal/library/match_test.go` — `It 1990` MUST NOT match `It 2017` (FR-005); a match still stands when either side carries no year; two folders sharing a key both come back (collisions are real per spec Edge Cases)
- [ ] T005 [P] Failing tests for the snapshot in `server/internal/api/library_test.go` against the fake `syno.Client` — parents deduplicated across enabled sources so a shared parent is listed once (FR-007); a `ListFolder` error yields an `Empty` index that matches nothing rather than an error (FR-009); a reading younger than 5 minutes is reused and an older one is rebuilt (FR-010); invalidation after a send (FR-008) and after a source's parents change or a source is added/disabled/removed (FR-008a)

### Implementation

- [ ] T006 Implement `Key(name) (key, year string)` in `server/internal/library/match.go` — split the trailing year/range, drop a leading article and bracketed extras, lowercase, then keep only `unicode.IsLetter`/`unicode.IsDigit` runes (never an ASCII range)
- [ ] T007 Implement `Entry`, `Index`, and `Lookup(catalogTitle, mediaType)` in `server/internal/library/match.go` per [data-model.md](./data-model.md) §1, including the `Empty` flag that distinguishes "nothing is there" from "we could not look"
- [ ] T008 Implement the snapshot build and its mutex-guarded 5-minute cache in `server/internal/api/library.go` — collect the distinct `MoviesParent`/`TVParent` set across enabled providers, list each once via `d.NAS.Do` + `ListFolder("/"+parent)`, and swallow every failure into an `Empty` index
- [ ] T009 Invalidate the snapshot after a successful send in `handleSourceSend` (`server/internal/api/source_handlers.go`) and from the provider create/update/delete handlers in `server/internal/api/source_multi.go` (FR-008, FR-008a)
- [ ] T010 Add `[library]=85` to the `floors` map in the "Coverage floor (critical packages)" step of `.github/workflows/build-test.yml` (floors are a ratchet — add, never remove). **Must come after T006/T007**: the floor step runs `go test ./internal/library/` and parses a coverage percentage, so adding it while the package has no test files turns CI red
- [ ] T011 [P] Add a `POST /__mock/library` control endpoint that seeds folders into the mock's tree, plus its test, in `server/internal/synomock/synomock.go` and `server/internal/synomock/synomock_test.go` — the mock's fixtures are hardcoded in `resetLocked()` today with no way to seed, so no story can be tested end-to-end without this

**Checkpoint**: matching and the snapshot are proven against fakes; user stories can begin.

---

## Phase 3: User Story 1 — See what you already have while browsing (Priority: P1) 🎯 MVP

**Goal**: Titles already on the NAS carry a marker in the Discover grid.

**Independent Test**: Seed a title folder on the mock NAS, open Discover, and find that title
carrying the marker while its neighbours do not.

### Tests for User Story 1

- [ ] T012 [P] [US1] Failing tests in `server/internal/api/source_handlers_test.go` — a search result for a seeded title carries `inLibrary: true`; an unseeded one omits the field; and **every** item omits it when the snapshot failed to build, with the search itself still returning `200` (FR-009)

### Implementation for User Story 1

- [ ] T013 [US1] Add `InLibrary bool \`json:"inLibrary,omitempty"\`` to `CatalogTitle` in `server/internal/source/source.go`, documented as API-layer-set like the existing `SourceID`/`SourceName`
- [ ] T014 [US1] Decorate `res.Items` from the snapshot in `handleSourceSearch` immediately before the response is written, in `server/internal/api/source_handlers.go` (mirrors `decorateTasks` in `task_ownership.go`)
- [ ] T015 [US1] Add `inLibrary?: boolean` to the `CatalogTitle` interface in `src/services/api.ts`, documented so absence and `false` are treated identically
- [ ] T016 [US1] Render the marker inside `.poster` in `src/views/tabs/BrowserPage.vue` — an `ion-icon` checkmark plus text on a `.badge.badge-have` modifier of the existing badge CSS, positioned `right` so it cannot collide with the `Soon` badge on the left, carrying an `aria-label` so the state is not colour-only (FR-012, FR-013)
- [ ] T017 [US1] Add `e2e/stateful/library.spec.ts` — seed a folder matching a mock catalog title, assert the marker on that card and its absence on others, and assert a same-name-different-year title is NOT marked (SC-004)

**Checkpoint**: User Story 1 is shippable on its own — the request's core value is delivered.

---

## Phase 4: User Story 2 — Know which seasons you already have (Priority: P2)

**Goal**: Opening a series shows which seasons are already on the NAS beside its download options.

**Independent Test**: Seed a series folder holding seasons 1 and 2, open that series, and see
those two marked and later seasons unmarked — in both the nested and the flat on-disk layout.

### Tests for User Story 2

- [ ] T018 [P] [US2] Failing contract test for `ListEntries` in `server/internal/syno/http_test.go`, beside the existing `TestFileStationBrowse` — directories and files come back from one call, and a missing path errors
- [ ] T019 [P] [US2] Failing tests for season detection in `server/internal/library/seasons_test.go` — nested (`Season 1`, `S01`, `Series 1`), flat (`Friends.S01E01.mkv` and the underscore-separated `X_Men_97_S01E01_1080p` form that a `\b`-bounded pattern would miss), a blank or unparseable name producing no spurious season, and `Files: 0` meaning "not counted" rather than "empty" (FR-016)
- [ ] T020 [P] [US2] Failing handler tests in `server/internal/api/library_handlers_test.go` — the `200` shape; `seasons: []` for a movie; **`400` for every escape attempt** (`../`, `/`, `\`, a control character, `.`, `..`, empty after trimming) asserting the value is rejected and NOT sanitised into a different folder (FR-025a); `409` with no source configured; `429` over the per-user bound (FR-025b); and `200` with `{inLibrary:false, seasons:[]}` — never a `5xx` — when the NAS read fails (FR-017). Also assert FR-025c: the route applies the same access rules as `GET /v1/source/title/{id}` and is not a way for a content-rating-restricted user to learn something that route would not already tell them

### Implementation for User Story 2

- [ ] T021 [US2] Add `Entry` and `ListEntries` to `server/internal/syno/client.go` and implement it in `server/internal/syno/http.go` using the already-allowlisted `SYNO.FileStation.List`/`list` with **no `filetype` parameter** (DSM defaults to all), then add it to the fake in `server/internal/api/fake_test.go`. Leave `ListFolder` untouched — the picker and the parent scan still want dirs only
- [ ] T022 [US2] Give the mock a file layer in `server/internal/synomock/synomock.go` — a `files` map alongside `folders`, honour the `filetype` parameter (`all`/`file`/`dir`) in `handleFileStation`, and extend `/__mock/library` to seed files
- [ ] T023 [US2] Implement season detection in `server/internal/library/seasons.go` for both layouts, reusing the non-alphanumeric-bounded `S01E05` form shared by `src/services/task-title.ts` and `server/internal/tasktitle/tasktitle.go` rather than inventing a second pattern
- [ ] T024 [US2] Implement `GET /v1/library/title` in `server/internal/api/library_handlers.go` and register it in the stateful branch of `server/internal/api/router.go` — validate-and-reject the client-supplied title (FR-025a), apply the per-user rate bound (FR-025b), resolve seasons from one `ListEntries` call with a bounded concurrent descent (stdlib `sync` only) for counts in the nested layout, and return no folder path (FR-025)
- [ ] T025 [US2] Add `api.libraryTitle(type, title)` to `src/services/api.ts` per [contracts/library-api.md](./contracts/library-api.md) §2
- [ ] T026 [US2] Mark present seasons in `src/components/SourceTitleModal.vue` — fetch in parallel with the existing `getSourceTitle()` call so a failure only hides the markers (FR-017), and place them using the existing `seasonNum()` from `src/services/quality-sort.ts` and the season grouping from spec 2005 rather than a second grouping mechanism
- [ ] T027 [US2] Extend `e2e/stateful/library.spec.ts` — assert the correct seasons are marked for a nested layout and for a flat layout, and that a movie shows ownership with no season breakdown (FR-018)

**Checkpoint**: User Stories 1 and 2 both work independently.

---

## Phase 5: User Story 3 — Guardrails against downloading it twice (Priority: P3)

**Goal**: Re-sending something already present asks for confirmation, and owned titles can be
hidden from the grid.

**Independent Test**: Send a title already present and confirm the prompt can be cancelled with
nothing sent; separately, enable the toggle and confirm owned titles leave the grid and the
setting survives a reload.

### Tests for User Story 3

- [ ] T028 [P] [US3] Failing round-trip test for `hide_owned` in `server/internal/store/source_repos_test.go`, and confirm `store_test.go:TestOpenRunsMigrations` still passes with the longer migration list
- [ ] T029 [P] [US3] Failing tests for `hideOwned` through `GET`/`PUT /v1/source/prefs` in `server/internal/api/source_handlers_test.go`, including that an omitted field leaves the stored value unchanged
- [ ] T030 [P] [US3] Failing tests in `src/composables/useSourceCatalog.test.ts` — owned titles are filtered out; the grid backfills further pages when the filter leaves it underfull (FR-023a); the backfill is bounded so an exhausted or heavily-owned catalog cannot spin

### Implementation for User Story 3

- [ ] T031 [US3] Append migration 0019 (`ALTER TABLE source_prefs ADD COLUMN hide_owned INTEGER NOT NULL DEFAULT 0;`) to `migrations` in `server/internal/store/schema.go` with the house `// 00NN — why` comment. Append only — never edit a shipped migration
- [ ] T032 [US3] Carry `hide_owned` through the existing `GetSourceViewFull`/`SaveSourceViewFull` pair in `server/internal/store/source_repos.go` — one read and one upsert for the whole Discover view, no new accessor pair
- [ ] T033 [US3] Add `hideOwned` to the prefs request/response shapes and handlers in `server/internal/api/source_handlers.go`
- [ ] T034 [US3] Add `hideOwned` to the client prefs type in `src/services/api.ts`
- [ ] T035 [US3] Filter owned titles in `useSourceCatalog.fetchPage()` at the same point `comingSoon` is already dropped, and add the bounded backfill to `loadMore()`, in `src/composables/useSourceCatalog.ts`
- [ ] T036 [US3] Add the hide-owned `ion-toggle` to `src/components/SourceFilterSheet.vue`, persisting through the prefs endpoint so it follows the user across devices (FR-024)
- [ ] T037 [US3] Add the confirmation to `send()` in `src/components/SourceTitleModal.vue` — an **`ion-alert`, never a native `confirm()`**, which would block the page — fired when a movie is present or the selected season is present, and not at all otherwise (FR-019, FR-021)
- [ ] T038 [US3] Extend `e2e/stateful/library.spec.ts` — cancelling the confirmation sends nothing **and consumes none of the user's daily download allowance** (FR-020); accepting proceeds; the toggle removes owned cards, keeps the grid filled, and survives a reload

**Checkpoint**: all three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T039 Add a regression test in `server/internal/api/source_handlers_test.go` proving FR-027: a send to a destination the user has no folder grant for is still refused with `403 destination_forbidden`, whether or not the title is marked as already present. Nothing else guards the requirement that this feature leaves the existing permission boundary untouched
- [ ] T040 Review every new code path for FR-026 — no folder name, file name, or path from the NAS in any log line, error payload, metric, **or panic**, across `server/internal/api/library.go`, `library_handlers.go`, and `server/internal/syno/http.go`
- [ ] T041 [P] Seed `make start` fixtures in `server/internal/synomock/synomock.go` whose folder names match mock catalog titles, so the marker is visible in development without hand-seeding
- [ ] T042 [P] Document the ownership behaviour and the `/__mock/library` seeding endpoint in `docs/DOWNLOAD-SOURCES.md`
- [ ] T043 Walk [quickstart.md](./quickstart.md) end to end, including the three failure paths (silent degradation with the NAS stopped, immediate recognition after a send, and the 5-minute staleness bound)
- [ ] T044 Run all five gates: `npm run build`, `npm run test:unit:coverage`, and in `server/`: `go build ./...`, `go vet ./...`, `go test ./...`, then `npm run test:e2e`
- [ ] T045 Set `**Status**:` to `in-review` in `specs/0008-show-which-discover/spec.md` and run `make roadmap` (CI's `Roadmap up to date` guard fails if it is stale)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: needs Setup — **blocks all three user stories**
- **User Stories (Phases 3–5)**: all need Phase 2. Then P1 → P2 → P3, or in parallel if staffed
- **Polish (Phase 6)**: needs the stories that are being shipped

### User Story Dependencies

- **US1 (P1)**: needs only Phase 2. Ships alone as the MVP
- **US2 (P2)**: needs only Phase 2. Adds the `ListEntries` capability, which nothing else uses
- **US3 (P3)**: needs Phase 2 for the ownership signal the confirmation and filter both read. Its store/prefs work (T028–T036) is independent of US1 and US2 and could land first if preferred

### Within Each Story

Failing tests → server model/logic → server endpoint → client type → client UI → e2e.

### Parallel Opportunities

- T003, T004, T005 are three different concerns and can be written together
- T011 (mock seeding) is independent of all matching work in Phase 2
- Within US2, T018/T019/T020 are three separate test files
- Within US3, T028/T029/T030 span store, handler, and client
- The three client type changes all touch `src/services/api.ts`, so none carries `[P]` — they are sequential by file, not by logic

---

## Implementation Strategy

**MVP is Phase 1 + Phase 2 + Phase 3 (US1).** That delivers exactly what was asked — a marker in
Discover showing what you already have — and is independently releasable. Phases 4 and 5 refine
it: US2 makes the signal actionable for series, US3 turns it from information into a guardrail.

Each phase ends at a checkpoint where the app builds, all gates pass, and the feature is coherent
for a user. Nothing half-wired ships.
