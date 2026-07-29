# Implementation Plan: Per-User Download Statistics and Richer Notifications

**Branch**: `feat/0006-per-user-download` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/0006-per-user-download/spec.md`

## Summary

Add durable, per-user download history (both Discover/catalog and directly-added
tasks), aggregate it into a new **Statistics** section in Settings (per-category
counts + average sizes + a historical downloads graph), and make push
notifications human-readable and — for admins/owner subscribed to everyone —
attributed to the user who added the download.

Technical approach: a new append-only `download_history` table written at
task-create time (authoritative for counts, so paused/canceled still count,
matching the daily-limit accounting) and backfilled with the real file size by
the existing completion watcher. A server-side port of the client's
`task-title.ts` produces readable notification titles; `notifyEvent` builds a
per-subscriber body that appends the owner's username only for all-scope
admin/owner subscribers. New read-only `/v1/stats/*` endpoints feed a hand-rolled
SVG chart and stat lists composed from stock Ionic components. Direct downloads
are categorized by a folder+extension heuristic that the user can override in the
new-task modal.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript / Vue 3 + Ionic (client)

**Primary Dependencies**: server — stdlib `net/http`, `modernc.org/sqlite` (no new
deps); client — `@ionic/vue`, `vue` (no new charting dependency — hand-rolled SVG)

**Storage**: the single SQLite volume (`DATA_DIR/synodl.db`); one new table
(`download_history`) via appended migration `0013`

**Testing**: Go `go test` (fake `syno.Client` + in-memory SQLite store); client
vitest (pure modules: title port lives client-side already, new date-bucketing +
size-averaging helpers get unit tests); Playwright e2e for the Statistics section
and the readable/attributed notification path via the mock DSM

**Target Platform**: single container serving the PWA at `/` and API at `/v1`

**Project Type**: web (Vue 3 PWA + Go service in one repo/image)

**Performance Goals**: Statistics endpoints answer from a single indexed table in
well under a second for a household-scale library (hundreds–low thousands of
rows); graph granularity switches happen client-side with no refetch

**Constraints**: no new runtime dependencies; stock Ionic UI (SVG chart is the one
justified bespoke widget); credential-safety invariants preserved (Principle III)

**Scale/Scope**: single NAS, a handful of household users, low-thousands of
download-history rows per year

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven Development** — ✅ Work is spec `0006`; pipeline being followed
  (specify → clarify → **plan** → tasks → analyze → taskstoissues → implement).
- **II. Test-Driven Development** — ✅ `tasks.md` will order failing tests first:
  store aggregation tests, watcher size-backfill test, the Go title-port table
  test, handler tests against the fake client, client vitest for bucketing/size
  helpers, and an e2e for the Statistics view + readable notification. This
  touches session/HTTP-handler and new persistence → unit tests are mandatory;
  new user-facing behavior → e2e added.
- **III. Custodial State & Credential Safety (NON-NEGOTIABLE)** — ✅ with care:
  - *One store, one volume*: the new `download_history` lives in the existing
    SQLite DB; no new datastore/volume.
  - *Download tasks are never persisted as task state*: `download_history` is an
    **append-only event log for attribution/statistics**, not a live task mirror —
    the NAS remains the source of truth for current task state, exactly as the
    existing `download_events` / `source_downloads` tables already do. The table
    stores no live status beyond a completion timestamp + final size.
  - *Secrets never leak*: notification payloads carry only the readable title and
    (for admins) a username — never URIs/sids/credentials. No secrets logged.
  - *DSM allowlist unchanged*: no new DSM API is called; the watcher already polls
    the allowlisted task list.
  - A **Credential-Safety Impact** section is included below, and because this
    spec touches stored data, **`/speckit-checklist` is REQUIRED** before
    `/speckit-implement` (see Gate sequencing).
- **IV. Offline-First Client Data** — ✅ Statistics are server-aggregated and
  fetched read-only; no IndexedDB store is added, so **no `DB_VERSION` bump**. The
  chart holds data in memory and re-buckets locally.
- **V. Quality Gates** — ✅ Plan targets green `npm run build`, `go build/vet/test`,
  vitest+floors, and e2e. Commit subjects for user-facing slices will be
  benefit-focused ("See per-user download stats and a history graph"; "Download
  alerts now show the title and who added it").
- **VI. Ionic-First UI** — ⚠️ One justified deviation: the historical graph is a
  hand-rolled inline **SVG** because Ionic has no chart primitive (user-confirmed:
  no new charting dependency). It is wrapped in stock Ionic (`ion-segment`,
  `ion-list`, `ion-select`, `ion-modal`) and uses existing `--app-*` theme tokens.
  Recorded in Complexity Tracking.
- **VII. Traceable, Auto-Closing Delivery** — ✅ `taskstoissues` will open issues;
  the PR into `main` will list `Closes #N`. `ROADMAP.md` already regenerated.

**Result**: PASS (one reasoned Principle VI deviation; checklist required for III).

### Credential-Safety Impact

- **What is stored & how protected**: `download_history` rows — `user_id`
  (FK, `ON DELETE CASCADE`), `source` (catalog/direct), `category`, `destination`
  and `task_name` (folder/file names, used only to correlate the completion for
  size backfill), `created_at`, `completed_at`, `size_bytes`, and an optional
  correlated `task_id`. No credentials, no URIs, no session ids. Same volume,
  same encryption posture as the rest of the DB (this table holds no secrets, so
  no per-column encryption is required).
- **What crosses to the NAS**: nothing new — the watcher already polls the
  allowlisted task list; no new DSM API.
- **What could appear in logs/errors**: only route + outcome, as today. The
  readable title and username in notifications are not logged. Destination/name
  columns are internal correlation data, never emitted to logs.
- **Why**: statistics and attribution require a durable, per-user, size-and-type
  record that the ephemeral live task list cannot provide; this mirrors the
  already-accepted `download_events` (counts) and `source_downloads` (metadata)
  precedent from spec 1013 / 0005.

## Project Structure

### Documentation (this feature)

```text
specs/0006-per-user-download/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── stats-api.md     # Phase 1 output — /v1/stats/* + notification payload
├── checklists/
│   └── requirements.md  # from /speckit-specify (spec quality)
└── tasks.md             # /speckit-tasks output (NOT created here)
```

### Source Code (repository root)

```text
server/internal/
├── store/
│   ├── schema.go              # + migration 0013 (download_history)
│   ├── history_repos.go       # NEW: AddDownloadHistory, CompleteDownloadHistory,
│   │                          #      StatsSummary, StatsDaily (+ _test.go)
│   └── repos.go               # (unchanged; user delete already cascades)
├── api/
│   ├── stats_handlers.go      # NEW: GET /v1/stats/summary, /v1/stats/timeseries (+ _test.go)
│   ├── router.go              # + register the two stats routes
│   ├── source_handlers.go     # + AddDownloadHistory(source=catalog) at send time
│   └── stateful_handlers.go   # + AddDownloadHistory(source=direct) + category from body/heuristic
├── tasktitle/
│   └── tasktitle.go           # NEW: Go port of src/services/task-title.ts (+ _test.go)
├── mediaclass/
│   └── mediaclass.go          # NEW: folder+extension → category heuristic (+ _test.go)
└── push/
    └── watcher.go             # readable title in notifyEvent; per-subscriber username;
                               # CompleteDownloadHistory size-backfill on completion

src/
├── services/
│   ├── api.ts                 # + getStatsSummary(), getStatsTimeseries() + DTOs
│   └── stats-buckets.ts       # NEW: pure day→week/month/year/all bucketing (+ .test.ts)
├── composables/
│   └── useStatistics.ts       # NEW: read-only loader (useDestinationPrefs pattern)
├── components/
│   ├── StatisticsModal.vue    # NEW: dive-in ion-modal wrapper (UserManagementModal pattern)
│   ├── StatisticsView.vue     # NEW: source/bucket segments, per-category stat list, user picker
│   ├── DownloadsChart.vue     # NEW: hand-rolled SVG bar chart (theme-token styled)
│   └── NewTaskModal.vue       # + category ion-select (Auto default) sent on create
├── utils/format.ts            # REUSE formatBytes for average sizes
└── views/tabs/SettingsPage.vue# + "Statistics" dive-in row (own for all; user picker for admins)

e2e/
└── statistics.spec.ts         # NEW: seed history via mock, assert stats + graph + gating
```

**Structure Decision**: Existing monorepo web layout (Go server + Vue client in
one repo). New server logic follows the one-file-per-area + sibling `_test.go`
convention; two new tiny pure packages (`tasktitle`, `mediaclass`) carry their own
table tests. Client follows the established composable + dive-in-modal patterns.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Hand-rolled SVG chart (Principle VI: Ionic-first) | Ionic ships no chart/graph primitive, and the feature requires a historical downloads graph with switchable buckets | A charting library was explicitly declined by the requester (adds a runtime dependency + bundle weight, against the project's minimal-dependency posture). The SVG is wrapped in stock Ionic and uses existing theme tokens, keeping it accessible and theme-correct. |
