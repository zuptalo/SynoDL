# Tasks: Show release year and readable genres on titles

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-07

Format: `[ID] [P?] [Story]` — `[P]` = parallelisable, `[F]` = foundational.
A failing test precedes the implementation that satisfies it (Principle II).

## Phase 1: Server — make both sources speak the same vocabulary

- **T001** `[F]` Add `Year string \`json:"year,omitempty"\`` to
  `source.CatalogTitle` — `server/internal/source/source.go`.
- **T002** `[F]` (FR-001) Failing test: a ZarFilm card with a year in its markup
  yields that year on the catalog title — `providers/zarfilm_test.go`.
- **T003** `[F]` Populate `Year` from the already-parsed `it.Year` —
  `providers/zarfilm.go`.
- **T004** `[F]` (FR-003, FR-004) Failing test: ZarFilm card genres come back as
  English slugs, using the same `parseGenreSlugs` mapping the facets use; an
  unmapped Farsi genre falls back to its own name rather than vanishing —
  `providers/zarfilm_test.go`.
- **T005** `[F]` Map card genres through the slug table —
  `providers/zarfilm.go`.
- **T006** `[P]` `[F]` (FR-001) Establish whether 30nama publishes a usable
  per-item year; populate `Year` if so, and record in the plan if not —
  `providers/nama30.go` + `nama30_test.go`.
- **T007** `[P]` `[F]` Assert 30nama card genres are already English slugs, so a
  regression there is caught — `providers/nama30_test.go`.

## Phase 2: Client — the two judgement calls, as tested pure modules

- **T008** `[F]` (FR-005) Failing tests for `genreLabel()`: slug wins; a
  hyphenated slug title-cases ("sci-fi" → "Sci-Fi"); an unmapped value falls back
  to the source's name; a raw value never reaches the user with hyphens —
  `src/services/genre-label.test.ts`.
- **T009** `[F]` Implement — `src/services/genre-label.ts`.
- **T010** `[F]` (FR-006, FR-011) Failing tests for `displayYear()`: a plausible
  year passes; one outside a sane range is dropped; a series range survives; the
  title-derived year is used only when the source field is empty; a year present
  in both places is not doubled — `src/services/release-year.test.ts`.
- **T011** `[F]` Implement — `src/services/release-year.ts`.

## Phase 3: User Story 1 — the card (P1) 🎯 MVP

- **T012** `[US1]` Add `year?: string` to the client `CatalogTitle` —
  `src/services/api.ts`.
- **T013** `[US1]` (FR-001, FR-002, FR-007, FR-008) Render the caption as
  rating · year · genre, suppressing the type while a type filter is active, on
  the existing line budget — `src/views/tabs/BrowserPage.vue`.
- **T014** `[US1]` e2e: with a type filter applied, a card shows a year and an
  English genre and does not repeat the type — `e2e/stateful/`.

**Checkpoint**: the grid tells titles apart.

## Phase 4: User Story 2 — the details header (P2)

- **T015** `[US2]` (FR-009, FR-010) Show the year beside type and rating, and
  render genres through `genreLabel()` so the sheet matches the card —
  `src/components/SourceTitleModal.vue`.
- **T016** `[US2]` e2e: opening a title shows the same year and genres its card
  showed — `e2e/stateful/`.

## Phase 5: Gates

- **T017** `npm run build` · **T018** `npm run test:unit:coverage` ·
  **T019** `cd server && go build ./... && go vet ./... && go test ./...` ·
  **T020** `npm run test:e2e` · **T021** `make roadmap`, status → `in-review`.

## Dependencies

Phase 1 and Phase 2 are independent of each other and both block Phase 3.
Phase 4 depends on Phase 2 only. `[P]` tasks touch disjoint files.
