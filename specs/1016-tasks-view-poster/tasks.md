# Tasks: Tasks view — posters, cleaner titles, Open in Discover

**Feature**: `specs/1016-tasks-view-poster` | **Branch**: `feat/1016-tasks-view-poster`

**Input**: [plan.md](./plan.md), [spec.md](./spec.md)

TDD for the server persistence (tests before/with the change). Stateful → Go integration tests, no
e2e. One GitHub issue per task.

## Phase 1: Setup
- [ ] T001 Baseline green on `feat/1016-tasks-view-poster`: `cd server && go test ./...`,
  `npm run test:unit`, `npm run build`.

## Phase 2: Foundational — persist poster_url + catalog_id (blocks US2/US3)
- [ ] T002 Store layer + round-trip test. In `server/internal/store/schema.go` append TWO migrations:
  `ALTER TABLE source_downloads ADD COLUMN poster_url TEXT NOT NULL DEFAULT '';` and
  `... ADD COLUMN catalog_id TEXT NOT NULL DEFAULT '';`. Add `PosterURL`/`CatalogID` to
  `store.SourceDownload` and to `SaveSourceDownload` (INSERT + upsert SET + args) and `SourceDownloads`
  (SELECT + Scan) in `source_repos.go`. Add a `SaveSourceDownload`/`SourceDownloads` round-trip test in
  `source_repos_test.go` asserting the new columns persist and read back (write test first).
- [ ] T003 Handler + enrichment + tests. In `server/internal/api/source_handlers.go` add
  `PosterURL`/`CatalogID` to `sourceSendReq` and pass them into the `SaveSourceDownload` call (catalog
  id from `body.TitleID` or an explicit `catalogId`). In `task_ownership.go` add
  `posterUrl`/`catalogId` to `taskView` and set them in the `if hasMedia {}` block. Extend
  `task_ownership_test.go` (`TestDecorateTasksMediaMetadata`) to assert poster/catalogId land on the
  view, and `source_handlers_test.go` (`TestSourceSendMovie`) to assert they persist.
- [ ] T004 [P] Client wiring for the new fields. Add `posterUrl?`/`catalogId?` to `src/types/task.ts`;
  in `src/services/api.ts` include `posterUrl` in the `sendSource` payload (and confirm `titleId` is
  sent as the catalog id); in `src/components/SourceTitleModal.vue` pass `props.title.posterUrl` (and
  id) into the `sendSource` call.

## Phase 3: User Story 1 — Cleaner titles (P1)
- [ ] T005 [US1] In `src/services/task-title.ts`, run the chosen title through `splitYear`
  (`src/services/title-year.ts`) to strip a trailing year and also return the parsed year; write/adjust
  `src/services/task-title.test.ts` first (year stripped from title; parsed year returned; titles
  without a trailing year untouched).
- [ ] T006 [US1] In `src/components/TaskItem.vue`, render the cleaned heading and show the year on the
  metadata line using `task.year ?? parsedYear`, so plain (non-catalog) tasks still show a year.

## Phase 4: User Story 2 — Poster thumbnail (P1)
- [ ] T007 [US2] In `src/components/TaskItem.vue`, add a left-aligned poster thumbnail bound to
  `task.posterUrl` with a neutral placeholder (film icon) when empty or on image error; keep row
  height/layout tidy in light and dark.

## Phase 5: User Story 3 — Open in Discover (P2)
- [ ] T008 [US3] In `src/composables/useSourceCatalog.ts` add a `pendingOpen` handoff (a
  `CatalogTitle | null` ref + optional pending query) and a `requestOpen(title|null, query?)`; in
  `src/views/tabs/BrowserPage.vue` consume it (open `SourceTitleModal` via `openTitle`, or `setQuery`
  for the search fallback) when the tab is shown, then clear it.
- [ ] T009 [US3] In `src/components/TaskDetailModal.vue` add an "Open in Discover" action shown only
  when `task.mediaType` is set: if `task.catalogId` exists, build a minimal `CatalogTitle`
  (id/title/type/posterUrl/imdbScore + defaults), `requestOpen(it)` and navigate to `/tabs/browser`;
  else `requestOpen(null, task.title)` (search fallback) and navigate.

## Phase 6: User Story 4 — Hide torrent-only fields (P3)
- [ ] T010 [US4] In `src/components/TaskDetailModal.vue`, gate the Uploaded, Upload-speed, and
  Peers/seeders rows on `task.type === 'bt'`.

## Phase 7: Polish
- [ ] T011 Gate: `cd server && go build ./... && go vet ./... && go test ./...`; `npm run build`;
  `npm run test:unit:coverage`. Flip Status to `in-review`; `make roadmap`; bump version; open PR.
- [ ] T012 Manual verify on the deployed build: send a Discover download → poster + single year on the
  row; Open in Discover opens the exact title; a plain HTTP task shows no torrent stats and a
  placeholder poster.

## Dependencies
- T002 → T003 → T004 (server persistence chain; then client fields). US2 (T007) and US3 (T009) require
  T004. US1 (T005/T006) and US4 (T010) are independent and may go in parallel with the server work.
- T011/T012 last.

## Implementation strategy
MVP = US1 + US2 (cleaner titles + posters). US3 (Open in Discover) and US4 (field guard) layer on.
All ship in one PR into `main`.
