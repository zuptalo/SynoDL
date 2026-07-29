---

description: "Task list for spec 0006 — Per-User Download Statistics and Richer Notifications"
---

# Tasks: Per-User Download Statistics and Richer Notifications

**Input**: Design documents from `specs/0006-per-user-download/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/stats-api.md

**Tests**: REQUIRED. Constitution Principle II mandates TDD — every phase writes
its failing tests before implementation (Red → Green → Refactor). Server logic gets
Go unit tests against the fake `syno.Client` / in-memory store; pure client modules
get vitest; new user-facing behavior gets an e2e spec.

**E2E note**: e2e can't run on this dev machine (macOS 12) — e2e tasks are authored
here and validated in CI.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1–US4 map to the spec's user stories

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: shared constants; no new dependencies (server stays zero-dep, client adds no charting lib).

- [x] T001 Add shared media-category + source constants in `server/internal/store/history_repos.go` (new file header): `SourceCatalog="catalog"`, `SourceDirect="direct"`, and the six category strings (`movie`,`series`,`anime`,`music_video`,`music`,`other`); export a `ValidCategory(string) bool` used by handlers to sanitize input.
- [x] T002 [P] Add matching TS category/source unions in `src/services/api.ts` (`Category`, `StatsSource`) so client and server agree on the enum spelling (`musicVideo` ↔ `music_video` mapping documented inline).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the durable `download_history` table + its store layer. BLOCKS US2, US3, US4 (all read/write history). US1 does NOT depend on this phase.

**⚠️ CRITICAL**: complete before starting US2/US3/US4.

- [x] T003 [US-FOUND] Write failing store tests in `server/internal/store/history_repos_test.go`: (a) `AddDownloadHistory` inserts a size-less row; (b) counts include size-less (paused/canceled) rows; (c) `StatsSummary` averages over completed rows only and omits categories with no completed rows; (d) grouping by user_id/source/category; (e) `StatsDaily` groups by UTC date and returns per-day counts; (f) deleting a user cascades away their history rows.
- [x] T004 [US-FOUND] Write failing test for `CompleteDownloadHistory` in the same `_test.go`: matches the oldest size-less row by `(destination, task_name)`, sets `size_bytes`/`completed_at`/`task_id`, returns `false` when nothing matches, and never double-assigns (a second call for the same pair matches the next row, not the already-completed one).
- [x] T005 [US-FOUND] Add migration `0013` (the `download_history` table + two indexes) to the `migrations` array in `server/internal/store/schema.go`, exactly as in `data-model.md` (append only — never edit a shipped migration).
- [x] T006 [US-FOUND] Implement `DownloadHistory`/`UserSourceStats`/`DayCount` types and `AddDownloadHistory` + `CompleteDownloadHistory` in `server/internal/store/history_repos.go` to pass T003(a,b)/T004.
- [x] T007 [US-FOUND] Implement `StatsSummary(userIDs)` and `StatsDaily(userIDs, source)` in `history_repos.go` (SQL GROUP BY; `AVG` over `size_bytes IS NOT NULL`) to pass T003(c,d,e).

**Checkpoint**: history can be recorded, completed, and aggregated; user-delete cascade verified.

---

## Phase 3: User Story 1 - Readable, attributed notifications (Priority: P1) 🎯 MVP

**Goal**: notification bodies show the human-readable title; all-scope admin/owner subscribers also see who added the download. No dependency on the history table.

**Independent Test**: send a release-scene-named Discover download as a regular user; the user's notification shows the clean title, the admin's shows title + "added by <user>"; a non-admin never sees another user's name.

### Tests for User Story 1 ⚠️ (write first, must fail)

- [x] T008 [P] [US1] Write `server/internal/tasktitle/tasktitle_test.go` porting the cases from `src/services/task-title.test.ts` (underscore/`SE_RE` boundary, dotted names, URL-path episode, movie folder title, raw-name fallback).
- [x] T009 [P] [US1] Write a watcher notification test in `server/internal/push/watcher_test.go` (or extend existing): asserts the pushed body uses the readable title; an "any"-scope admin viewing another user's task gets `… · added by <username>`; the owner viewing their own does NOT get the username; a non-admin ("own") is unaffected and never receives another user's task.

### Implementation for User Story 1

- [x] T010 [P] [US1] Implement `server/internal/tasktitle/tasktitle.go` — pure `Title(name, destination, uri string) (title, episode string)` mirroring `src/services/task-title.ts` (folder-title derivation + `SxxEyy`); pass T008.
- [x] T011 [US1] In `server/internal/push/watcher.go` `poll`, compute the readable body via `tasktitle.Title(t.Name, t.Destination, t.URI)` (combine title + episode) and pass it to `notifyEvent` instead of raw `t.Name`.
- [x] T012 [US1] In `notifyEvent`, move payload construction inside the per-subscriber loop; when the effective scope is `any`, the download is attributed (`ownerUserID != 0`), and `ownerUserID != sub.UserID`, append ` · added by <username>` (resolve username once via `GetUserByID`, memoized per call). Pass T009. Keep the credential-safety invariant (no URIs/sids in the payload).
- [x] T013 [US1] Improve catalog owner resolution for notifications: in the watcher completion/added branches, when a name-claim doesn't attribute, fall back to `source_downloads` owner-by-destination (mirrors `decorateTasks`) so catalog completions attribute correctly for the username line. Add/extend a watcher test case.

**Checkpoint**: US1 shippable on its own — readable + attributed notifications, no schema change.

---

## Phase 4: User Story 2 - Per-user statistics in Settings (Priority: P2)

**Goal**: record catalog downloads to history, backfill sizes on completion, expose `/v1/stats/summary`, and show per-category counts + average sizes in a new Statistics section (own-only for regular users, all-users for admins). Depends on Phase 2.

**Independent Test**: with completed catalog downloads of known type/size, the Statistics section shows matching counts + averages; a regular user sees only their own; a category with no completed downloads shows "—".

### Tests for User Story 2 ⚠️ (write first, must fail)

- [x] T014 [P] [US2] Write `server/internal/api/stats_handlers_test.go` for `GET /v1/stats/summary`: non-admin is scoped to their own row regardless of params; admin/owner receives every user; `avgSizeBytes` is `null` for categories with no completed rows; legacy/stateless mode → 403.
- [x] T015 [P] [US2] Extend `server/internal/api/source_handlers_test.go`: a catalog send writes one `download_history` row per selected file with `source=catalog` and `category=media_type`, `size_bytes` NULL at send.
- [x] T016 [P] [US2] Extend `server/internal/push/watcher_test.go`: a task finishing backfills the matching catalog history row's size via `CompleteDownloadHistory`; a fast finish/no-match leaves size NULL without error.

### Implementation for User Story 2

- [x] T017 [US2] In `server/internal/api/source_handlers.go` (the send handler, after `AddDownloadEvents`/`SaveSourceDownload`), call `AddDownloadHistory` once per selected file with `source=catalog`, `category=body.Type`, `destination=dest`, and `task_name` = the resolved link's file name. Pass T015.
- [x] T018 [US2] In `server/internal/push/watcher.go` completion branch (and the first-sight-already-finished case), call `store.CompleteDownloadHistory(t.Destination, t.Name, t.Size, now)`; log a debug no-match, never an error. Pass T016.
- [x] T019 [US2] Implement `handleGetStatsSummary` in `server/internal/api/stats_handlers.go` (resolve visible user set from session role; non-admin → self; build the `users[]` payload per `contracts/stats-api.md`). Pass T014.
- [x] T020 [US2] Register `GET /v1/stats/summary` in `server/internal/api/router.go` (behind `requireUser`, stateful-only).
- [x] T021 [P] [US2] Add `getStatsSummary()` + `StatsUserSummary`/`StatsCategoryStat` DTOs to `src/services/api.ts`.
- [x] T022 [P] [US2] Create `src/composables/useStatistics.ts` (module refs + guarded loader, modeled on `useDestinationPrefs.ts`) exposing summary state + a `load()`; degrade gracefully on 403/offline.
- [x] T023 [US2] Create `src/components/StatisticsView.vue` — per-category counts + average sizes (reusing `formatBytes` from `src/utils/format.ts`); "—" for null averages; admin user picker (`ion-select`) that refetches summary for the chosen user or all. Stock Ionic only.
- [x] T024 [US2] Create `src/components/StatisticsModal.vue` dive-in wrapper (copy the `UserManagementModal.vue` shape) hosting `StatisticsView`.
- [x] T025 [US2] Add a "Statistics" dive-in row to `src/views/tabs/SettingsPage.vue` — visible to all signed-in users in stateful mode (own stats); the admin user picker inside is what widens it to all users.

**Checkpoint**: US1 + US2 both work; catalog stats visible with correct gating.

---

## Phase 5: User Story 4 - Track directly-added downloads by source (Priority: P2)

**Goal**: record manually-added (torrent/URL) downloads to history under `source=direct` with a category (heuristic + user override), and let statistics filter by source (catalog / direct / all). Depends on Phase 2; extends US2's view.

**Independent Test**: add a direct download (optionally choosing a category), let it finish; it appears under the direct source attributed to the adder with the chosen/detected category and real size, is absent from catalog figures, does not count against the daily limit, and the combined view sums both sources.

### Tests for User Story 4 ⚠️ (write first, must fail)

- [x] T026 [P] [US4] Write `server/internal/mediaclass/mediaclass_test.go`: folder-parent classification (movies→movie, tv/series→series, anime→anime, music→music, music-video→music_video), audio-extension fallback → music, ambiguous → other.
- [x] T027 [P] [US4] Extend `server/internal/api/stateful_handlers_test.go`: a direct URI add and a torrent-file add each write a `source=direct` history row attributed to the user; an explicit `category` wins; an absent/`auto`/invalid category falls back to `mediaclass.Classify`; the daily limit is NOT decremented for direct adds.

### Implementation for User Story 4

- [x] T028 [P] [US4] Implement `server/internal/mediaclass/mediaclass.go` — pure `Classify(destination, fileName string) string`; pass T026.
- [x] T029 [US4] Add an optional `Category` field to `createTaskJSON` and read a `category` form value in `server/internal/api/stateful_handlers.go`; sanitize via `store.ValidCategory` (invalid/`auto`/absent ⇒ heuristic).
- [x] T030 [US4] In both create paths of `stateful_handlers.go` (URI and torrent-file), call `AddDownloadHistory` with `source=direct`, chosen-or-classified `category`, `destination`, and `task_name` (the `titleHint(uri)` / `header.Filename`). Do NOT touch `download_events` (limit stays catalog-only). Pass T027.
- [x] T031 [P] [US4] Add an optional `category` param to `createTaskURIs`/`createTaskFile` in `src/services/api.ts`.
- [x] T032 [US4] Add a category `ion-select` (default **Auto**, options movie/series/anime/music video/music/other) to `src/components/NewTaskModal.vue`; send the chosen value on create.
- [x] T033 [US4] Add a source segment (`ion-segment`: Catalog / Direct / All) to `src/components/StatisticsView.vue`; compute the combined-source view client-side from the summary's two source objects.

**Checkpoint**: US1 + US2 + US4 work; direct downloads tracked and filterable, limit unaffected.

---

## Phase 6: User Story 3 - Historical downloads graph (Priority: P3)

**Goal**: a hand-rolled SVG graph of downloads over time with day/week/month/year/all-time buckets and source filtering, re-bucketing client-side without refetch. Depends on Phase 2; extends US2's view.

**Independent Test**: with downloads across several days, the graph shows correct per-bucket counts; switching granularity/source re-aggregates instantly with consistent totals; empty periods show as zero.

### Tests for User Story 3 ⚠️ (write first, must fail)

- [x] T034 [P] [US3] Write `src/services/stats-buckets.test.ts`: daily counts aggregate correctly into week/month/year/all-time using local-time boundaries; empty buckets are zero-filled across the covered range; totals are conserved across granularities.
- [x] T035 [P] [US3] Extend `server/internal/api/stats_handlers_test.go` for `GET /v1/stats/timeseries`: per-day counts with zero-filled gaps; non-admin forced to self; admin may pass `userId`/`all` and `source`.

### Implementation for User Story 3

- [x] T036 [US3] Implement `handleGetStatsTimeseries` in `server/internal/api/stats_handlers.go` (daily counts for the visible scope/source, zero-filled from first record to today) and register `GET /v1/stats/timeseries` in `router.go`. Pass T035.
- [x] T037 [P] [US3] Add `getStatsTimeseries(params)` + `StatsDaily` DTO to `src/services/api.ts`; extend `useStatistics.ts` to load and cache the daily series.
- [x] T038 [P] [US3] Implement `src/services/stats-buckets.ts` — pure day→week/month/year/all-time aggregation with zero-fill; pass T034.
- [x] T039 [US3] Create `src/components/DownloadsChart.vue` — inline SVG bar chart styled with `--app-*` tokens (light/dark aware), with an accessible text summary; renders the bucketed series.
- [x] T040 [US3] Wire the bucket segment (`ion-segment`: day/week/month/year/all) into `src/components/StatisticsView.vue`, re-bucketing client-side (no refetch) and feeding `DownloadsChart`.

**Checkpoint**: all four stories functional and independently testable.

---

## Phase 7: Polish & Cross-Cutting

- [x] T041 [P] Add e2e `e2e/statistics.spec.ts`: seed catalog + direct downloads via the mock DSM, assert summary numbers, source filtering, graph bucketing, admin-vs-regular gating, and one readable/attributed notification (validated in CI).
- [x] T042 [P] Update coverage-floor allowlists if `stats-buckets.ts` / `tasktitle` cases push pure-module coverage (ratchet up, never down) — see `vitest` config + server floors.
- [x] T043 `quickstart.md` verification: the API/behavior path is validated end-to-end by the integration tests (real HTTP router + store + mock DSM: `stats_handlers_test.go`, `stats_history_write_test.go`, `watcher_test.go`), and the client compiles/typechecks + unit-tests. A local stateful click-through via `make start` isn't possible (dev runs stateless, and stateful mode expects an HTTPS NAS while the mock is HTTP) — the live UI click-through happens on the deployed stateful instance after release (Keel).
- [x] T044 [P] Full gate pass: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`.
- [x] T045 Bump `package.json` version (`npm run release:minor` — new user-facing feature) so the merge cuts a release; ensure the feature commits carry benefit-focused "What's new" subjects (e.g. "See per-user download stats with a history graph", "Download alerts now show the title and who added it").

---

## Dependencies & Execution Order

- **Phase 1 (Setup)**: no deps.
- **Phase 2 (Foundational)**: after Setup. **Blocks US2, US3, US4.** Does NOT block US1.
- **US1 (Phase 3)**: after Setup only — independent MVP, can run in parallel with Phase 2.
- **US2 (Phase 4)**: after Foundational.
- **US4 (Phase 5)**: after Foundational; extends `StatisticsView.vue` created in US2 (do US2 first, or coordinate the shared file).
- **US3 (Phase 6)**: after Foundational; also extends `StatisticsView.vue` (sequence after US2).
- **Polish (Phase 7)**: after the desired stories.

### Shared-file sequencing (avoid conflicts)

- `server/internal/push/watcher.go` — touched by US1 (T011–T013) then US2 (T018): sequential.
- `src/components/StatisticsView.vue` — created in US2 (T023), extended by US4 (T033) and US3 (T040): sequential.
- `src/services/api.ts` — additive edits across US2/US3/US4 (T021, T031, T037): coordinate but low-risk.

### Parallel opportunities

- T008 & T010 (tasktitle test+impl) run alongside all of Phase 2.
- Within Foundational: T003/T004 (tests) before T005–T007 (impl).
- Pure helpers `tasktitle`, `mediaclass`, `stats-buckets` are independent and [P].
- Server handler tests (T014, T015, T016, T035) are [P] with each other.

---

## Implementation Strategy

- **MVP** = Phase 1 + Phase 3 (US1). Readable/attributed notifications ship with no
  schema change — deployable and demoable on its own.
- **Increment 2** = Phase 2 + US2: durable history + the Statistics summary.
- **Increment 3** = US4 (direct-source tracking) then US3 (historical graph),
  each extending the same Statistics view.
- Commit after each task or logical group; verify each story independently at its
  checkpoint before moving on.

## Notes

- TDD is enforced: the `⚠️` test tasks in every phase are written and made to FAIL
  before their implementation tasks.
- `/speckit-checklist` is REQUIRED (Principle III — this spec stores data) and
  `/speckit-analyze` must be clean before `/speckit-implement`.
