# Implementation Plan: Show which Discover titles you already have

**Branch**: `feat/0008-season-episode-ownership` | **Date**: 2026-09-04 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0008-show-which-discover/spec.md`

## Summary

US1 shipped in 0.3.0 and is **wrong in a way the spec licensed**: it treats a folder
existing as proof of ownership. The destination folder is created before a download
starts, and metadata tools write into it independently, so titles read as owned while
holding nothing watchable. This plan corrects that and builds the two remaining stories.

Three changes, in dependency order:

1. **Ownership becomes evidence-based.** A title is present only when a *video file*
   exists beneath its folder. This needs `SYNO.FileStation.List` with `filetype=file` —
   the same already-allowlisted API, one new method on the client.
2. **A third state: downloading.** A title with an active download task writing into its
   folder is reported as downloading, not owned. Derived from tasks already polled; costs
   no NAS read.
3. **Season and episode detail.** Series report which seasons are present and which
   episode numbers each holds, never claiming completeness.

The cost model is what makes this viable: the cheap name index survives as a **candidate
filter**, and only the handful of on-screen titles that match a folder name are verified.
A title matching nothing costs no NAS call at all.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3.5 + Ionic 8 (client)
**Primary Dependencies**: stdlib `net/http`; `modernc.org/sqlite`; Vite; no new dependency
**Storage**: none added — the index is in-memory and per-instance; the NAS stays the source of truth
**Testing**: `go test` against fakes + `synomock`; vitest for pure client modules; Playwright under `e2e/stateful/`
**Target Platform**: single container (server + built PWA), iOS/Android/desktop browsers
**Project Type**: web (Go service + Vue PWA in one repo)
**Performance Goals**: Discover feels no slower than today (SC-009); a page of results adds at most a handful of NAS listings, typically 1–5
**Constraints**: pod runs at 192Mi; no state added; ownership never asserted before it is verified
**Scale/Scope**: libraries of several hundred title folders; ~20–40 catalog items per Discover page

## Constitution Check

| Principle | Gate | Status |
|---|---|---|
| I. Spec-Driven Development | Spec amended and clarified before planning | **PASS** — 5 clarifications recorded 2026-09-04 |
| II. Test-Driven Development | Failing tests precede implementation; mock must model DSM faithfully first | **PASS with a precondition** — see Complexity Tracking |
| III. Custodial State & Credential Safety | No new DSM API; no new stored state; no folder or file names in logs | **PASS** — `filetype=file` widens what an allowlisted API *returns*, so `/speckit-checklist` MUST re-run before implementation |
| IV. Offline-First Client Data | Ownership is server-derived and never persisted client-side | **PASS** — no new IndexedDB store, no `DB_VERSION` bump |
| V. Quality Gates | build + unit + vet + go test + e2e all green | **PASS** — enforced per task |
| VI. Ionic-First UI | Third marker state uses existing Ionic primitives and `--app-*` tokens | **PASS** |
| VII. Traceable Delivery | One issue per task, `Closes #N` in the PR | **PASS** — via `/speckit-taskstoissues` |

**Principle III detail.** No `SYNO.*` API is added: `SYNO.FileStation.List` is already
allowlisted and already registered for discovery. What changes is that it is now called
with `filetype=file`, so the server *reads file names* where it previously read only
directory names. That is a genuine widening of the read and the reason the checklist must
re-run — but it is not an allowlist change and needs no new spec-level API decision.

## Project Structure

### Documentation (this feature)

```
specs/0008-show-which-discover/
├── spec.md              # amended 2026-09-04
├── plan.md              # this file
├── research.md          # Phase 0 — decisions and rejected alternatives
├── data-model.md        # Phase 1 — Index, TitleOwnership, SeasonPresence
├── contracts/           # Phase 1 — /v1/library/title, decorated search results
├── quickstart.md        # Phase 1 — how to exercise it against synomock
└── checklists/          # requirements.md + security.md (security.md re-runs)
```

### Source Code (repository root)

```
server/internal/
├── syno/            # + ListFiles(ctx, sid, path) — same API, filetype=file
├── library/         # + video-extension test, episode extraction from file names
├── api/             # library.go: candidate filter + lazy per-folder verification
│                    # source_handlers.go: decorate with owned | downloading
│                    # library_handlers.go: GET /v1/library/title
└── synomock/        # PRECONDITION: real file tree, replacing the flat uploads map

src/
├── services/        # api.ts: ownership state on CatalogTitle; library title lookup
├── views/tabs/      # BrowserPage.vue: owned vs downloading markers
└── components/      # SourceTitleModal.vue: season + episode presence
```

## Complexity Tracking

| Deviation | Why needed | Why the simpler option is insufficient |
|---|---|---|
| `synomock` gains a real file tree before any test is written | Both upload bugs shipped in 0.3.0 hid behind the mock being *more permissive than DSM* — it ignored the API discovery `query` filter and accepted `_sid` from the body. A flat `uploads` map cannot represent `Show/Season 01/ep.mkv`, so season tests written against it would assert a shape DSM never produces | Seeding fixtures into the existing map would let every season test pass while the real listing shape is untested — the exact failure mode that cost a release |
| Ownership verification is lazy rather than eager | Verifying every title folder is one NAS listing per folder — hundreds per refresh on a real library | A full scan either hammers the NAS every 5 minutes or forces a long staleness window (FR-010a caps recognition at 5 minutes) |
| Three ownership states rather than two | FR-001b: a part-fetched title has a video file on disk and would otherwise read as owned | A boolean cannot distinguish "skip this" from "wait for this" — the advice differs |
