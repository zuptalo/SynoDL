# Implementation Plan: Show which Discover titles you already have

**Branch**: `feat/0008-show-which-discover` | **Date**: 2026-09-03 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0008-show-which-discover/spec.md`

## Summary

Read the children of the movies/TV parent folders the download sources are configured to write
into, match those folder names against the titles Discover shows, and surface the answer as a
poster marker, a season breakdown in the title sheet, a confirmation before re-sending, and a
hide-owned toggle.

The approach leans on an existing property of the system rather than on fuzzy release-name
parsing: **`handleSourceSend` already names each destination folder `sanitizeFolderName(title)`
under the source's configured parent**, so a folder's name is derived from the very catalog
string being compared against. Content SynoDL downloaded matches essentially exactly; a
normalisation pass (case, punctuation, articles, bracketed extras, year form) catches everything
that arrived by other routes.

No new DSM API is authorized — `SYNO.FileStation.List` is already allowlisted and already drives
the destination picker. The one genuine widening is reading a browsed title's folder *contents*
(files, not just subdirectories), needed for season detection.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: **None added.** The new matching and season-parsing code is pure Go
standard library (`strings`, `regexp`, `unicode`, `sync`). Client uses the existing Ionic/Vue
stack.

**Storage**: SQLite, single volume. Exactly **one additive `ALTER TABLE`** (`source_prefs`
gains `hide_owned`). The library snapshot itself is **never persisted** — it lives in process
memory with a 5-minute TTL (FR-010), so the NAS stays the source of truth and no migration
carries a copy of the user's library.

**Testing**: `go test` against the fake `syno.Client` (`api/fake_test.go`) and `httptest` +
`synomock` (`syno/http_test.go`); Vitest for pure client modules and `useSourceCatalog`;
Playwright against the **stateful** e2e stack (`e2e/stateful/`, ports 5275 / 8283 / 8294).

**Target Platform**: Single container serving the PWA and the API on one origin.

**Project Type**: Web application — Vue PWA + Go service in one repo, one image.

**Performance Goals**: A catalog search served from a warm snapshot adds **zero** NAS
round-trips (SC-009). A cold or expired snapshot costs one `ListFolder` per *distinct* parent —
typically 2, not 2×N, because sources commonly share parents. Season detail costs one
`ListEntries` on the opened title's folder, plus bounded concurrent reads only for the nested
layout.

**Constraints**: Principle III. The widened read is confined to the operator's configured
parents; folder and file names never reach logs, errors, or metrics (FR-026); every read failure
degrades to "nothing is present" rather than surfacing an error (FR-009).

**Scale/Scope**: A parent folder may hold thousands of title folders — one DSM listing each,
held as a map. One new pure Go package, one new server file plus one endpoint, one additive
migration, and three client touch-points (grid card, title sheet, filter sheet).

## Constitution Check

*GATE: passed before Phase 0; re-checked after Phase 1 design — still passing.*

| Principle | Status | Notes |
|---|---|---|
| I. Spec-Driven Development | **Pass** | `make spec` → 0008 → specify → clarify → this plan. No code before `tasks.md`. |
| II. Test-Driven Development | **Pass** | The matching rules are a pure package written test-first from a table (year collisions, articles, non-Latin scripts). `ListEntries` gets a `synomock` contract test before use. Snapshot build, degrade-on-error, and send-invalidation are tested against the fake `syno.Client` before wiring. New user-facing behaviour adds an `e2e/stateful/` spec. |
| III. Custodial State & Credential Safety | **Pass, after closing five gate findings** | No new DSM API; `SYNO.FileStation.List` is already allowlisted. The read widens from directory names to file names inside a browsed title's folder — disclosed in the spec's Credential-Safety Impact, and the reason `/speckit-checklist` is mandatory here. Nothing about NAS contents is persisted. `checklists/security.md` found five requirement gaps — client-input path escape, an unbounded lookup rate, panics missing from the no-logging rule, invalidation on configuration change, and the content-rating interaction — all now closed as FR-008a, FR-025a–c, and an amended FR-026. The least-privilege tension (an instance-wide signal) is tabled under Complexity Tracking. |
| IV. Offline-First Client Data | **Pass** | No new IndexedDB object store, so **no `DB_VERSION` bump**. The hide-owned flag lives server-side with the rest of the Discover view state, so it follows the user across devices (FR-024). |
| V. Quality Gates Are the Definition of Done | **Pass** | All five gates run. Commit subjects are release-note copy, e.g. `feat(discover): see at a glance what you already have`. |
| VI. Ionic-First UI | **Pass** | Marker is an `ion-icon` + text on the existing `.badge` pattern; confirmation is an `ion-alert`; the toggle is an `ion-toggle` in the existing `SourceFilterSheet`. No bespoke widgets. |
| VII. Traceable, Auto-Closing Delivery | **Pass** | Tasks become issues; the PR lists `Closes #N` for each. `make roadmap` after the status line moves. |

**Mock-DSM dev parity**: preserved and extended. `synomock` currently models directories only
(`folders map[string][]string`, every entry `isdir:true`) and honours no `filetype`. It gains a
file layer and a `/__mock/library` seeding endpoint, so `make start` and the e2e suite still need
no real hardware.

## Project Structure

### Documentation (this feature)

```
specs/0008-show-which-discover/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0 — matching rules, layouts, freshness, prior art in-repo
├── data-model.md        # Phase 1 — snapshot/entry shapes, the one migration
├── contracts/
│   └── library-api.md   # Phase 1 — inLibrary on search results + GET /v1/library/title
├── quickstart.md        # Phase 1 — seeding a fake library and seeing the badge
└── checklists/
    ├── requirements.md  # written by /speckit-specify
    └── security.md      # REQUIRED (Principle III) — written by /speckit-checklist
```

### Source Code (repository root)

```
server/
├── internal/library/            # NEW — pure, no I/O, carries a coverage floor
│   ├── match.go                 #   Key(), Index, Lookup — normalisation + year agreement
│   ├── match_test.go
│   ├── seasons.go               #   season detection from dir names and episode file names
│   └── seasons_test.go
├── internal/syno/
│   ├── client.go                # + ListEntries on the Client interface, + Entry type
│   └── http.go                  # + ListEntries (SYNO.FileStation.List/list, no filetype)
├── internal/synomock/
│   └── synomock.go              # + file layer, honour filetype, + POST /__mock/library
├── internal/store/
│   ├── schema.go                # + migration 0019: source_prefs.hide_owned
│   └── source_repos.go          # hide_owned through the existing prefs accessors
└── internal/api/
    ├── library.go               # NEW — snapshot build + TTL cache + invalidation
    ├── library_test.go
    ├── library_handlers.go      # NEW — GET /v1/library/title
    ├── library_handlers_test.go
    ├── router.go                # + the one route (stateful only)
    ├── source_handlers.go       # decorate search results; invalidate after a send
    └── fake_test.go             # + ListEntries on the fake client

src/
├── services/api.ts              # + inLibrary on CatalogTitle; + libraryTitle(); hideOwned pref
├── composables/useSourceCatalog.ts   # hide-owned filter + backfill (FR-023a)
├── views/tabs/BrowserPage.vue   # the poster marker
├── components/SourceTitleModal.vue   # season markers + the confirm prompt
└── components/SourceFilterSheet.vue  # the hide-owned toggle

e2e/stateful/
└── library.spec.ts              # NEW — badge, season markers, confirm, hide-owned
```

**Structure Decision**: The matching logic goes in a **new pure package**
(`server/internal/library`) rather than inside `api/`, for the same reason `syno` and `config`
are separable: it is the part with real edge cases, it needs no NAS, and it can therefore carry a
coverage floor and be exhaustively table-tested. All I/O and caching stay in `api/library.go`,
following the existing `decorateTasks` pattern in `api/task_ownership.go`.

## Implementation phases

**A — Matching core (pure, no I/O).** `library.Key`, `library.Index`, `library.Lookup`, and the
season parsers, written test-first. Includes the Go port of the `S01E05` form already shared
between `src/services/task-title.ts` and `server/internal/tasktitle/tasktitle.go`.

**B — Reading the NAS.** `syno.Entry` + `ListEntries` on the `syno.Client` interface, the
`HTTPClient` implementation, the `synomock` file layer and `filetype` handling, and the contract
test between them. Then `api/library.go`: build a snapshot from the distinct parents across
enabled providers, cache it under a mutex with a 5-minute TTL, invalidate after a successful
send **and whenever a source's parents change or a source is added, disabled, or removed**
(FR-008a), and degrade to an empty snapshot on any error.

**C — The marker (User Story 1, P1).** `InLibrary` on `source.CatalogTitle`, set in
`handleSourceSearch` just before the response is written; mirrored on the client type; rendered
as a second badge on the poster. **This phase alone is a shippable MVP.**

**D — Season detail (User Story 2, P2).** `GET /v1/library/title`, resolving both on-disk
layouts from one `ListEntries` call, with bounded concurrent descent for episode counts in the
nested layout. The handler validates-and-rejects the client-supplied title rather than sanitising
it (FR-025a) and sits behind the existing rate limiter (FR-025b) — both are gate findings from
`checklists/security.md`, not optional hardening. Wired into `SourceTitleModal` using the existing `seasonNum()` from
`quality-sort.ts` and the season grouping from spec 2005.

**E — Guardrails (User Story 3, P3).** The `ion-alert` confirmation in the modal's `send()`;
migration 0019 plus the `hide_owned` field on the existing `/v1/source/prefs` shape; the toggle in
`SourceFilterSheet`; the filter and backfill in `useSourceCatalog.fetchPage()`, at the same point
where `comingSoon` is already filtered out.

**F — End-to-end.** `/__mock/library` seeding, the `e2e/stateful/library.spec.ts` scenarios, and
a `make start` fixture whose folder names match mock catalog titles so the marker is visible in
development.

Phases C, D and E are independently shippable in that order, matching the spec's P1/P2/P3.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| The ownership signal is **instance-wide**, not scoped to the user's folder grants — so a user without a grant on `movie/` can still learn a title exists there. This sits in tension with Principle III's least-privilege clause. | The parents are the operator's own configuration and identical for every user; the fact being communicated ("this instance already has this title") is inherently instance-level. Scoping it per user would mark a title as absent for one user and present for another when it is the same folder, which is precisely the confusion the feature exists to remove. | Building a per-user snapshot filtered by folder grants was rejected: it multiplies NAS reads by the number of users, produces contradictory answers for the same title, and buys little — the leak is the existence of a title under a folder the operator configured, and it grants no new capability, because sending is still refused by `destinationAllowed` (FR-027). |
| Reading **file names** inside a browsed title's folder, where SynoDL previously read only directory names. | Season-level detail (User Story 2) cannot be derived any other way for content SynoDL did not download itself — the flat layout puts seasons only in file names. | Deriving seasons from `download_history` was rejected: it only knows what SynoDL sent, which excludes exactly the pre-existing content this feature is for. |

Neither is a new dependency; the server's dependency set is unchanged.

## Risks

- **False positives are the one unacceptable failure.** A title wrongly marked as owned makes a
  user skip something they wanted. Mitigated by the year-agreement rule (FR-005) and a matching
  table driven from real folder-name shapes; the tests treat a false positive as the priority
  case, not an afterthought.
- **A very large parent folder** returns thousands of entries in one DSM listing. Accepted: it is
  one call every 5 minutes, and the result is a map. If listings prove to need paging, that is a
  contained change inside `ListFolder`'s caller.
- **Folder-name collisions.** Two distinct catalog titles can sanitize to the same folder name, so
  both mark as present. This already happens on send (both would share one destination), so the
  feature reports the system's real behaviour rather than introducing a new inconsistency. Noted
  in the spec's Edge Cases.
- **Snapshot staleness after an out-of-band change** is bounded at 5 minutes by FR-010a, which was
  the user's explicit choice over always-fresh (too slow) and once-per-session (too stale).
- **Ionic dialog discipline**: the confirmation must be an `ion-alert`, never a native
  `confirm()`, which would block the page.
