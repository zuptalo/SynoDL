# Implementation Plan: Tasks view — posters, cleaner titles, Open in Discover

**Branch**: `feat/1016-tasks-view-poster` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/1016-tasks-view-poster/spec.md`

## Summary

Extend the existing per-download catalog metadata (spec 1013: media type / year / score stored in
`source_downloads`, matched to NAS tasks by destination) with a **poster URL** and **catalog id**, and
use them to enrich the Tasks tab:

- **US1 (client)**: strip the trailing year from the row heading; keep the year on the metadata line
  (stored `year`, else parsed from the name so plain tasks don't lose it).
- **US2 (server + client)**: persist `poster_url` + `catalog_id` at send-time; render the poster as a
  left thumbnail on the row with a neutral placeholder when absent.
- **US3 (client)**: an "Open in Discover" action in the task detail — opens the exact title (via the
  stored catalog id, handed to the Browser tab through shared catalog state) or falls back to a
  Discover search of the title.
- **US4 (client)**: show upload total / upload speed / peers / seeders only for `bt` tasks.

## Technical Context

**Language/Version**: Go 1.26 (server); TypeScript / Vue 3 + Ionic (client)
**Primary Dependencies**: stdlib net/http + SQLite (server); Ionic Vue + vue-router (client)
**Storage**: SQLite `source_downloads` table — TWO append-only ALTER migrations add `poster_url`,
`catalog_id` (both `TEXT NOT NULL DEFAULT ''`)
**Testing**: Go integration tests (store round-trip + `decorateTasks` enrichment + send-handler
persistence); vitest unit for the title-strip helper. NOT e2e (stateful; stateless harness can't
configure a source — see the e2e-stateless-harness constraint)
**Target Platform**: single container; k3s via Keel
**Project Type**: web application (Go backend + Vue PWA)
**Constraints**: append-only migrations, backward compatible; credential boundary preserved
**Scale/Scope**: ~4 server files + migrations + tests; ~5 client files + 1 unit test

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven** — ✅ spec `1016` drives it.
- **III. Custodial State & Credential Safety (NON-NEGOTIABLE)** — the change ADDS persisted data, so
  the gate is engaged and satisfied as follows (see the checklist `checklists/security.md`):
  - New columns hold **non-sensitive** per-download metadata: a public catalog poster URL (the same
    CDN image Discover already renders) and the catalog title id. **No** credentials, session ids,
    OTP, or signed download URIs are stored or logged.
  - Data lives in the **existing** encrypted SQLite volume, following the exact spec-1013 pattern
    (`source_downloads`, keyed by destination). No new volume, no schema outside that table.
  - **No** new outbound API and **no** widening of the DSM/source allowlist — poster/id are already
    in hand client-side from the catalog; we only carry them on the existing `/v1/source/send`.
  - Migrations are append-only and default empty, so existing rows and older clients are unaffected.
- **TDD** — ✅ store round-trip + enrichment + send-persistence Go tests written before/with the
  server change; a vitest unit test for the year-strip helper.
- **V. Release-note subject** — ✅ user-facing `feat(tasks)` plain-language subject.
- **Checklist gate** — a `checklists/security.md` is included because the change touches stored data
  (constitution's broadened trigger), even though no secret/allowlist surface changes.

No violations → no Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/1016-tasks-view-poster/
├── spec.md
├── plan.md
├── checklists/{requirements.md,security.md}
└── tasks.md
```

### Source Code (repository root)

```text
# Server
server/internal/store/schema.go                 # +2 append-only ALTER migrations (poster_url, catalog_id)
server/internal/store/source_repos.go           # SourceDownload{+PosterURL,+CatalogID}; Save/Read cols
server/internal/store/source_repos_test.go       # store round-trip test for the new columns
server/internal/api/source_handlers.go          # sourceSendReq{+posterUrl,+catalogId}; persist them
server/internal/api/task_ownership.go           # taskView{+posterUrl,+catalogId}; set in decorate
server/internal/api/task_ownership_test.go       # extend metadata enrichment test
server/internal/api/source_handlers_test.go      # assert send persists poster/catalogId

# Client
src/types/task.ts                               # Task{+posterUrl?,+catalogId?}
src/services/api.ts                             # SourceSearch/send: pass posterUrl (+ titleId already sent)
src/services/task-title.ts                      # strip trailing year; return parsed year
src/services/task-title.test.ts                  # unit test the strip
src/components/TaskItem.vue                      # poster thumbnail + placeholder; clean title; year on meta
src/components/TaskDetailModal.vue              # "Open in Discover" action; hide bt-only fields for non-bt
src/composables/useSourceCatalog.ts             # pendingOpen handoff (open-by-title-object / search)
src/views/tabs/BrowserPage.vue                  # consume pendingOpen → openTitle()/setQuery()
src/components/SourceTitleModal.vue             # send: include posterUrl in the sendSource payload
```

**Structure Decision**: Reuse the spec-1013 `source_downloads` persistence path end to end; the only
new plumbing is two columns threaded store → enrich → task JSON, plus a client thumbnail, an in-app
cross-tab open handoff, and a detail-view field guard.

## Complexity Tracking

No constitution violations — section intentionally empty.
