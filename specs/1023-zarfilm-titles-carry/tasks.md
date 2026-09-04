# Tasks: ZarFilm titles carry an IMDb link and a synopsis

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Phase 1: Foundational

- [ ] T001 Capture two page fixtures for the ZarFilm metadata block (movie + series) in `server/internal/source/providers/testdata/zarfilm/movie_meta.html` and `series_meta.html`, preserving the site's real element and class structure and its real IMDb anchor, with the synopsis body replaced by a short placeholder so the fixture tests structure rather than the site's editorial copy
- [ ] T002 Add `IMDbID` and `Plot` to `source.TitleDetail` in `server/internal/source/source.go`, with a comment recording that unlike `Ownership`/`Seasons` these are set by the driver and never by the handler

## Phase 2: User Story 1 — read what a ZarFilm title is about (P1)

**Goal**: A ZarFilm sheet shows the title's synopsis, right-to-left where the text is Persian.

**Independent test**: Open a ZarFilm movie and a ZarFilm series; each sheet shows a synopsis paragraph; a title with no synopsis renders as today.

- [ ] T003 [US1] Add failing tests for `parsePlot` in `server/internal/source/providers/zarfilm_parse_test.go`: reads the synopsis from the movie fixture and the series fixture (FR-002, FR-004); returns one copy where the page repeats it in the narrow-layout block (FR-005); returns empty for a whitespace-only or punctuation-only synopsis (FR-006); returns empty for the existing full-page fixtures, which carry no synopsis block (FR-010); does not panic on truncated markup
- [ ] T004 [US1] Implement `parsePlot` in `server/internal/source/providers/zarfilm_parse.go` — first non-empty of `div.plot` then `div.mobile_plot`, extracted with the existing `text()` walker so markup and entities resolve to text (FR-007)
- [ ] T005 [US1] Populate `td.Plot` in `zarfilm.Title()` for both the series and the movie branch in `server/internal/source/providers/zarfilm.go`, from the body already fetched (FR-002)
- [ ] T006 [P] [US1] Add `plot` to the client `TitleDetail` interface in `src/services/api.ts`
- [ ] T007 [US1] Merge detail metadata behind the catalog entry in `src/components/SourceTitleModal.vue`: `info` prefers the catalog value and falls back to the detail value (FR-008)
- [ ] T008 [US1] Add `dir="auto"` to the synopsis paragraph in `src/components/SourceTitleModal.vue` (FR-009)

## Phase 3: User Story 2 — follow a ZarFilm title through to IMDb (P2)

**Goal**: The rating on a ZarFilm sheet links out to IMDb, exactly as a 30nama one does.

**Independent test**: Open a ZarFilm title whose page links to IMDb; the header rating is a link to that IMDb title.

- [ ] T009 [US2] Add a failing test asserting `parseIMDbID` reads the real IMDb anchor from each new metadata fixture in `server/internal/source/providers/zarfilm_parse_test.go` (FR-003, FR-004)
- [ ] T010 [US2] Populate `td.IMDbID` in `zarfilm.Title()` for both branches in `server/internal/source/providers/zarfilm.go`
- [ ] T011 [P] [US2] Add `imdbId` to the client `TitleDetail` interface in `src/services/api.ts`

## Phase 4: Cross-cutting

- [ ] T012 Add a driver-level test in `server/internal/source/providers/zarfilm_test.go` asserting `Title()` returns both values for a movie and a series, and returns download options unchanged for a page carrying neither (FR-010, FR-011)
- [ ] T013 Run the gates: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`
- [ ] T014 Set the spec `**Status**:` to `in-review` and run `make roadmap`
- [ ] T015 Commit as `feat(discover): read what a ZarFilm title is about before downloading it`, push, open the PR, merge, and confirm k3s picks up the new image

## Dependencies

T001 → T003/T009. T002 → T005/T010. T005 → T007. US1 and US2 are independent of each other once T001/T002 land; both touch `Title()` and `api.ts`, so those edits are sequenced rather than parallel.
