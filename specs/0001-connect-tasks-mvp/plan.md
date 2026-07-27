# Implementation Plan: Connect to Download Station, view tasks, and add downloads

**Branch**: `claude/synology-nas-pwa-client-s8x3m0` | **Date**: 2026-07-27 | **Spec**: [spec.md](spec.md)

## Summary

Deliver the MVP on top of the bootstrap scaffold: the scaffold already ships
login (basic), a polling task list, settings/logout, the stateless Go proxy
with its mock DSM, and the hermetic e2e harness. This spec completes the five
user stories: hardened login incl. OTP flow (US1 — largely present, needs e2e
coverage), the live list (US2 — present, needs field-level e2e), add-task with
URL extraction, torrent upload, and folder picker (US3 — new), task actions
(US4 — new), and the filter/sort sheet with persisted state (US5 — new).

## Technical Context

- **Language/Version**: TypeScript 5.6 (Vue 3.5 + Ionic 8, Vite 6) client; Go 1.26 stdlib server (already complete for this spec's API surface).
- **Primary Dependencies**: no new runtime dependencies. UI composed from stock Ionic (`ion-modal` sheets, `ion-item-sliding`, `ion-action-sheet`, `ion-checkbox`, `ion-radio`).
- **Storage**: IndexedDB `settings` store — existing `session` row + new `taskFilter` row.
- **Testing**: vitest for the three new pure modules (`task-sort`, `url-detect`, `syno-errors`) added to the coverage allowlist; Playwright e2e specs per story against the mock DSM.
- **Target Platform**: mobile-first PWA, chromium-tested; server side untouched except no changes needed.
- **Performance Goals**: list render + sort/filter over hundreds of tasks stays instant (pure array ops); poll interval 3 s (SC-003 ≤ 5 s).
- **Constraints**: no server-side state; no new DSM APIs (the allowlist already covers everything this spec needs).

## Constitution Check

| Principle | Gate | Status |
|---|---|---|
| I. Spec-Driven | numbered spec, pipeline order, analyze clean before implement | **PASS** — this artifact set; analyze run before implementation. |
| II. TDD | failing tests before implementation; e2e per user-facing story; coverage ratchet | **PLANNED** — tasks.md orders RED tasks first per story; `task-sort`/`url-detect`/`syno-errors` join the vitest allowlist; each story lands an e2e spec. |
| III. Stateless, Credential-Free Proxy | no server state, no secret logging, allowlist only | **PASS (by design)** — zero server changes; source credentials/unzip password ride existing create params and are never persisted; spec carries the Credential-Safety Impact section. |
| IV. Offline-First Client Data | idb writes via wrapper, DB_VERSION discipline | **PASS** — `taskFilter` is a new row in the existing `settings` store; no schema change, no version bump needed. |
| V. Quality Gates | build/vet/tests/coverage/e2e green; conventional commits | **PASS (planned)** — the full gate list closes the task set. |
| VI. Ionic-First UI | stock components + `--app-*` tokens | **PASS (planned)** — sheets/modals/sliding items/checkboxes are all stock Ionic; no bespoke widgets. |
| VII. Traceable Delivery | roadmap current; issues per task | **PASS (partial)** — roadmap regenerated on Status changes. `taskstoissues` deferred: this bootstrap session pushes to a session branch without a PR; issues are opened when the branch goes to PR. |

**Result: PASS.** No principle requires a waiver.

## Project Structure

```
src/
  services/task-sort.ts        # NEW pure: term filter + sort + status filter
  services/url-detect.ts       # NEW pure: extract downloadable URLs from text
  services/syno-errors.ts      # NEW pure: ApiError code → user message
  components/TaskFilterSheet.vue  # NEW: term/sort/status sheet (ion-modal)
  components/NewTaskModal.vue     # NEW: URLs + torrent + options sheet
  components/FolderPickerModal.vue# NEW: shares → subfolder drill-down
  components/TaskItem.vue         # EXTEND: sliding pause/resume + delete
  views/tabs/TasksPage.vue        # EXTEND: FAB, filter button, wired sheets
  views/LoginPage.vue             # EXTEND: use syno-errors map (message parity)
  composables/useTaskFilter.ts    # NEW: persisted filter state (settings store)
e2e/
  connect.spec.ts   # US1   tasks-list.spec.ts # US2   add-task.spec.ts # US3
  task-actions.spec.ts # US4   filter-sort.spec.ts # US5
vitest.config.ts    # coverage allowlist += the three new pure modules
```

**Structure Decision**: pure logic lives in `src/services/*` so the coverage
gate applies; views stay thin Ionic composition.

## Complexity Tracking

No violations — no new dependencies, no server changes, no schema bumps.
