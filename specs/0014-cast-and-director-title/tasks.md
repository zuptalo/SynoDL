# Tasks: Who made it — cast and director on a title

**Input**: Design documents from `/specs/0014-cast-and-director-title/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/person-photo.md](./contracts/person-photo.md)

**Tests**: REQUIRED. Constitution Principle II mandates TDD — every test task in a
phase precedes the implementation it covers, and must fail before that
implementation lands.

**Organization**: by user story, so each is independently shippable. US1 is the
MVP and must stay green with every photograph path disabled.

## Format

`- [ ] [TaskID] [P?] [Story?] Description with file path`

`[P]` = touches files nothing else in the phase touches, so it can run in
parallel.

---

## Phase 1: Setup

- [x] T001 Read the Docker Scout report for the current `zuptalo/synodl` tag and apply every vulnerability that has a fix version (Go module bumps via `go get pkg@fixed && go mod tidy` in `server/`, or the base image in `Dockerfile`), noting any "no fix available" ones for the PR body — the supply-chain gate in the constitution's Development Workflow
- [x] T002 Add the mock-IMDb base override pair: `server/internal/source/providers/imdbbase_dev.go` (`//go:build sourcemock`, reads `IMDB_MOCK_BASE`) and `server/internal/source/providers/imdbbase_prod.go` (`//go:build !sourcemock`, returns `""`), mirroring `mockbase_dev.go`/`mockbase_prod.go` verbatim in shape and comment intent

---

## Phase 2: Foundational (blocks every user story)

**Purpose**: the shared types and the mock fixtures every later phase tests against.

- [x] T003 Add `Person` (with `Ref string \`json:"-"\``) and the `PersonResolver` interface to `server/internal/source/source.go`, with the comment explaining why `Ref` must never reach the wire (the `QualityOption.ReleaseName` precedent)
- [x] T004 Add `Cast`, `Directors`, `Creators`, `Writers` and `Year` to `source.TitleDetail` in `server/internal/source/source.go`, all `omitempty`, with a comment stating that an absent field means "the source publishes none" and is never `[]`
- [x] T005 [P] Capture `server/internal/source/providers/testdata/tn_single.json` from the shapes in research R1: a movie (two directors, three writers, every cast image under `/none/`) and a series (`director: null`, `creator: null`, one real person image, one `/none/`)
- [x] T006 [P] Capture `server/internal/source/providers/testdata/zarfilm/credits.html` — a `single_casts` block with ستارگان, کارگردان, and a کشور group that must be ignored
- [x] T007 [P] Capture `server/internal/source/providers/testdata/zarfilm/person_real.html`, `person_placeholder.html` (portrait under `/wp-content/themes/`) and `person_noimdb.html` (no `.linktoimdb`) per research R2
- [x] T008 Add the mock IMDb to `server/internal/synomock/imdbmock.go`: `GET /mockimdb/name/{nm}/` serving a page with `og:image` → `/mockimg/{nm}.jpg`, one id deliberately without one, `GET /mockimg/{nm}.jpg` serving bytes, and `POST /__mock/imdb/{up,down}` to make it refuse — registered from `server/internal/synomock/synomock.go`
- [x] T009 Add `GET /mocksrc/tn/api/v1/action/single/id/{id}` to `server/internal/synomock/sources.go`, returning the T005 shape for the mock's own title ids
- [x] T010a Add a length bound for values taken from a source — name, character, image URL — applied in `server/internal/source/source.go` before a `Person` leaves a driver, with a test in `server/internal/source/source_test.go` proving an oversized value is truncated rather than stored or sent (FR-030a)
- [x] T010 Add the credits block to the `zar` mock title page and serve `/mocksrc/zar/{actor,director}/{slug}/` person pages (one real portrait, one theme stand-in, one with no IMDb link) in `server/internal/synomock/sources.go`

**Checkpoint**: `go build ./...` and `go vet ./...` clean; mocks serve the new routes.

---

## Phase 3: User Story 1 — See who is in it, without leaving (P1) 🎯 MVP

**Goal**: names, characters and crew on the sheet, from both sources, with no
photograph machinery involved at all.

**Independent test**: open a title from each mock source and see the cast names,
character names and crew — with the mock IMDb switched off.

### Tests (write first, must fail)

- [x] T011 [P] [US1] `server/internal/source/providers/nama30_test.go`: `single/id/{id}` is parsed into cast (name, character, imdb id, billing order) and directors/creators/writers; `null` cast, `null` director and a missing response all yield empty slices, not errors
- [x] T012 [P] [US1] `server/internal/source/providers/nama30_test.go`: a failing/slow `single` call still returns the download options — `Title()` never fails on it
- [x] T013 [P] [US1] `server/internal/source/providers/zarfilm_parse_test.go`: `parseCredits` against `credits.html` returns cast and directors, ignores the کشور group, and yields no character names
- [x] T014 [P] [US1] `server/internal/api/source_handlers_test.go`: a title with no people serialises with no `cast`/`directors`/`creators`/`writers` keys at all; caps (20 cast, 10 per crew role) are enforced

### Implementation

- [x] T015 [US1] Add the `single/id/{id}` call to `server/internal/source/providers/nama30.go`, issued **in parallel** with the existing `download/id/{id}` call, with its failure swallowed into "no people" (FR-006); decode cast/director/creator/writer per research R1
- [x] T016 [US1] Add `parseCredits` to `server/internal/source/providers/zarfilm_parse.go`: walk `div.single_casts > div.stars`, match the Persian labels ستارگان → cast and کارگردان → directors, ignore every unrecognised label, and record each link's path as the person's `Ref`
- [x] T017 [US1] Wire `parseCredits` into `zarfilm.Title()` in `server/internal/source/providers/zarfilm.go` — no extra request; the page is already fetched
- [x] T018 [US1] Apply the per-role caps and de-duplication within a role (FR-004c, FR-007) in both drivers
- [x] T019 [P] [US1] Add the `Person` type and the four optional arrays to the client's title-detail type in `src/services/source.ts`
- [x] T020 [US1] Create `src/components/SourceCredits.vue`: `ion-list-header` per non-empty role in the fixed order cast → director → creator → writers, `ion-avatar` + `ion-label` tiles in a horizontally scrolling row, `dir="auto"` on every name and character, existing `--app-*` tokens only
- [x] T021 [US1] Render `SourceCredits` **below** the download options in `src/components/SourceTitleModal.vue` (FR-001a), showing nothing when every role is empty
- [x] T022 [P] [US1] `e2e/stateful/discover-credits.spec.ts`: with the mock IMDb **down**, open a title from each mock source and assert names, characters and role headings render and the download options are unaffected

**Checkpoint**: US1 is shippable. Faces are absent; nothing is broken.

---

## Phase 4: User Story 5 — Nobody renders as a broken image (P2)

**Goal**: the tile that has no photograph is a designed state, not a gap. Built
before the photo paths so every later phase has something correct to fall back to.

### Tests (write first, must fail)

- [x] T023 [P] [US5] `src/services/person.test.ts`: `initials()` returns up to two leading letters for Latin and non-Latin names, handles single names and extra whitespace, and returns `''` for a name with no letters
- [x] T024 [P] [US5] Add `src/services/person.ts` to the vitest coverage allowlist in `vitest.config.ts` — a ratchet up (Principle II)

### Implementation

- [x] T025 [US5] Add `initials(name)` to `src/services/person.ts`
- [x] T026 [US5] Render the initials tile in `src/components/SourceCredits.vue` — same size, shape and alignment as a photographed tile — used when no image source exists and on the `<img>` `error` event (FR-031, FR-032)
- [x] T027 [P] [US5] Extend `e2e/stateful/discover-credits.spec.ts`: a cast with no photographs anywhere renders a uniform row of initials tiles, verified in both themes

---

## Phase 5: User Story 2 — Faces, not a list of names (P1)

**Goal**: the photograph a source already has, shown, and the stand-in it
publishes instead, rejected.

### Tests (write first, must fail)

- [x] T028 [P] [US2] `server/internal/source/providers/nama30_test.go`: an image URL under `/none/` yields an empty `PhotoURL`; a `/person/` URL yields the `medium_webp` rendition
- [x] T029 [P] [US2] `server/internal/source/providers/zarfilm_parse_test.go`: `parsePersonPage` returns the portrait from `person_real.html`, no portrait from `person_placeholder.html` (theme path), and no IMDb id from `person_noimdb.html`

### Implementation

- [x] T030 [US2] Reject the `/none/` stand-in and select the `medium_webp` rendition in `server/internal/source/providers/nama30.go` (FR-013)
- [x] T031 [US2] Add `parsePersonPage` to `server/internal/source/providers/zarfilm_parse.go`: `.linktoimdb a[href]` → IMDb id (strict `nm` shape), `.inner_profile img` → portrait, rejecting any portrait under `/wp-content/themes/`
- [x] T032 [US2] Render the source photograph through the existing `/v1/source/image` proxy in `src/components/SourceCredits.vue`
- [x] T033 [P] [US2] Extend `e2e/stateful/discover-credits.spec.ts`: a person with a real source photo shows it; a person with a stand-in does not

---

## Phase 6: User Story 6 — The same actor in the next title is already known (P2)

**Goal**: the cache, before anything that would hammer a third party without it.

### Tests (write first, must fail)

- [x] T034 [P] [US6] `server/internal/store/person_repos_test.go`: put/get round-trips for both tables; an expired row reads as a miss; the row cap drops the oldest; a "none" answer is distinguishable from an absent row
- [x] T035 [P] [US6] `server/internal/people/people_test.go`: concurrent lookups of the same unresolved person cause exactly ONE outbound call (FR-028); an LRU hit makes no store read; TTLs differ for found and missing
- [x] T036 [US6] `server/internal/store/store_test.go` / `migrations_golden_test.go`: migration 39 applies to a fresh and to an upgraded database, and the golden checksum list is extended

### Implementation

- [x] T037 [US6] Add migration 39 (the two tables plus their indexes, `IF NOT EXISTS` throughout, with the comment explaining why) to `server/internal/store/schema.go`
- [x] T038 [US6] Append the new checksum to `migrationGolden` in `server/internal/store/migrations_golden_test.go` (run `go test ./internal/store/ -run TestMigrationsAreAppendOnly` to obtain it)
- [x] T039 [US6] Create `server/internal/store/person_repos.go`: `GetPersonPhoto`/`PutPersonPhoto`, `GetSourcePerson`/`PutSourcePerson`, read-time expiry, and the every-256th-write prune (expired first, then oldest beyond 20 000 per table)
- [x] T040 [US6] Create `server/internal/people/people.go`: the LRU, the hand-rolled single-flight, the 4-token outbound concurrency gate, and the store-backed resolve path with the TTLs from `data-model.md`
- [x] T041 [P] [US6] Extend `e2e/stateful/discover-credits.spec.ts`: open two titles sharing a cast member and assert the second causes no new outbound lookup (assert via the mock's request count)

---

## Phase 7: User Story 3 — A missing face is filled in from IMDb (P2)

**Goal**: the fallback, bounded and non-blocking.

### Tests (write first, must fail)

- [x] T042 [P] [US3] `server/internal/people/imdb_test.go`: `og:image` is found in a head-only read; the read stops at `</head>` and is capped at 256 KB; a page with no `og:image` yields "none"; a non-allowlisted host in `og:image` is refused
- [x] T043 [P] [US3] `server/internal/people/imdb_test.go`: the size rewrite turns `…_V1_FMjpg_UX1000_.jpg` into `…_V1_UX300_.jpg` and leaves a URL that does not match the shape alone
- [x] T044 [P] [US3] `server/internal/people/imdb_test.go` (`TestHostAllowlist`): only `www.imdb.com` and `m.media-amazon.com` over https are permitted; lookalikes (`imdb.com.evil.example`) are refused; this list is independent of the source image allowlist; and a redirect to a host outside the allowlist aborts the fetch rather than following it (FR-018b)
- [x] T045 [P] [US3] `server/internal/api/source_person_test.go` (`TestPersonPhoto`): a bad id is `400` before any outbound request; a person with no photograph is `404`; an upstream failure is `404`, never `5xx`; the rate limiter applies

### Implementation

- [x] T046 [US3] Create `server/internal/people/imdb.go`: the feature-local host rule, the bounded head scan for `og:image`, the rendition rewrite, and the `IMDB_MOCK_BASE` redirect through the T002 build-tag pair
- [x] T047 [US3] Create `server/internal/api/source_person.go`: `GET /v1/source/person/{imdbId}/photo` — `^nm\d{6,9}$` validation, its own 32 MB byte LRU, `Cache-Control: public, max-age=604800, immutable`, `X-Cache`, `404` for every miss — carrying the comment explaining why it is a third proxy and not a widened second one
- [x] T048 [US3] Register the route behind `limiter.Middleware` in `server/internal/api/router.go`, with a comment stating what the limiter is protecting here (a third party, not the NAS)
- [x] T049 [US3] Add `personPhotoSrc(imdbId)` to `src/services/api.ts` and use it in `src/components/SourceCredits.vue` as the second image source, after the source photo and before initials
- [x] T050 [P] [US3] Extend `e2e/stateful/discover-credits.spec.ts`: a person with no source photo gets one from the mock IMDb; with the mock IMDb down the same person renders initials and the sheet is otherwise identical (SC-006); and with the mock IMDb made SLOW, the sheet's names and download options render without waiting for it (FR-017, SC-002)

---

## Phase 8: User Story 4 — The tile is the way to their IMDb page (P2)

### Tests (write first, must fail)

- [x] T051 [P] [US4] `src/services/imdb-link.test.ts` (the file already exists — extend it): `imdbPersonUrl` accepts `nm` + digits and a canonical person URL, and returns `''` for a title id, a lookalike host, a `javascript:` URL and a path traversal
- [x] T052 [P] [US4] `server/internal/source/providers/zarfilm_test.go`: person refs are resolved to IMDb ids, bounded to 8 per title and 4 concurrent, the batch respects its deadline, and an unresolved person still appears with their name; and a router-level assertion that no route accepts a person REF from a client, so the source session is never spent on a client-supplied path (FR-014a)

### Implementation

- [x] T053 [US4] Add `imdbPersonUrl` to `src/services/imdb-link.ts` (strict, for the same reason `imdbUrl` is)
- [x] T054 [US4] Implement `ResolvePerson` on the zarfilm driver in `server/internal/source/providers/zarfilm.go`, and resolve a title's unidentified people in `server/internal/api/source_handlers.go` through the `people` package — bounded, deadlined, cache-first, degrading to "identity unknown" (FR-014)
- [x] T055 [US4] Make the tile a link in `src/components/SourceCredits.vue` where an IMDb id is known, opening the way the existing rating link does, and plainly not a link where it is not (FR-033)
- [x] T056 [P] [US4] Extend `e2e/stateful/discover-credits.spec.ts`: a tile with an id carries the IMDb person href; a tile without one is not a link

---

## Phase 9: User Story 7 — The detail sheet learns what it was missing (P3)

### Tests (write first, must fail)

- [x] T057 [P] [US7] `server/internal/source/providers/nama30_test.go`: `english_plot` wins, `persian_plot` is the fallback, the two are never concatenated, and `year`/`imdb` populate `TitleDetail`

### Implementation

- [x] T058 [US7] Populate `Plot`, `Year` and `IMDbID` from the `single` response in `server/internal/source/providers/nama30.go` (FR-036, FR-037)
- [x] T059 [US7] Prefer the detail `year` over the year split off the title string in `src/components/SourceTitleModal.vue`

---

## Phase 10: Polish & cross-cutting

- [x] T060 Confirm every item in `specs/0014-cast-and-director-title/checklists/security.md` still holds against the code as built — the checklist ran before implementation and closed six gaps in the spec (FR-014a, FR-018a, FR-018b, FR-030a); this is the re-read, not a re-run
- [x] T061 Audit that no name, character, or image URL reaches a log line, error payload or metric anywhere in the new code (FR-034), and that no upstream body is ever returned to a client
- [x] T062 [P] Verify the people section in light theme, dark theme and with RTL content on a phone viewport (FR-008), and that a cast longer than the viewport scrolls within its own row without the sheet or page scrolling sideways (FR-009)
- [x] T063 [P] Update `CLAUDE.md`'s source-driver description to mention that drivers now report people and that the person cache is the third thing in the store that is pure cache
- [x] T064 Set `**Status**: in-review` in `specs/0014-cast-and-director-title/spec.md` and run `make roadmap`
- [x] T065 Run the full gate: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`

---

## Dependencies

```
Phase 1 (setup) ─▶ Phase 2 (types + mocks) ─┬─▶ Phase 3  US1  names            ── MVP, shippable alone
                                            ├─▶ Phase 4  US5  initials tile     (independent of US1)
                                            └─▶ Phase 9  US7  plot/year/imdb    (independent)
                                                   Phase 3 ─▶ Phase 5  US2  source photos
                                                   Phase 5 ─▶ Phase 6  US6  cache
                                                   Phase 6 ─▶ Phase 7  US3  IMDb fallback
                                                   Phase 6 ─▶ Phase 8  US4  identity + links
                                            Phases 3–9 ─▶ Phase 10 polish
```

US6 (cache) deliberately precedes US3 (IMDb) and US4 (identity resolution): both
of those make outbound third-party requests, and shipping either without the cache
is the behaviour most likely to get an instance blocked.

## Parallel opportunities

- **Phase 2**: T005, T006, T007 are three independent fixture files.
- **Every phase's test tasks** are `[P]` against each other — different files.
- **Across phases**: Phase 4 (US5) and Phase 9 (US7) touch neither the drivers'
  people path nor the photo path, so either can proceed alongside Phase 3.
- **Client and server** within a phase are usually independent (e.g. T019/T020
  against T015–T018).

## Implementation strategy

Ship US1 first and confirm it stands with every photograph path disabled — that
is the property the whole spec is arranged around. Then add the fallback tile
(US5) so no later phase can regress into a broken image, then faces from the
sources (US2), then the cache (US6) **before** anything that talks to IMDb, then
the fallback (US3) and the links (US4). US7 rides along whenever convenient.
