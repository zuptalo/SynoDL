---
description: "Task list for 0002 live-task-updates"
---

# Tasks: Live task updates, task detail view, and download failure reasons

**Input**: Design documents in `/specs/0002-live-task-updates/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/
**Tests**: REQUIRED (constitution Principle II — TDD). Failing tests precede the implementation that satisfies them.

## Format: `[ID] [P?] [Story] Description`
- **[P]**: parallelizable (different files, no incomplete deps)
- **[Story]**: US1 live updates (P1), US2 failure reason (P1), US3 detail view (P2), US4 fallback (P2)

---

## Phase 1: Setup

- [ ] T001 Confirm branch `feat/0002-live-task-updates` is rebased on `main` (has the 0.0.2 hotfix), deps installed (`npm ci`), `go build ./...` green — baseline before changes.

## Phase 2: Foundational (blocking prerequisites for the user stories)

These carry the new data field end-to-end and the mock fixtures every story's tests rely on.

- [ ] T002 [P] Extend `server/internal/syno/http_test.go`: a fake-DSM task-list response containing `status_extra.error_detail` (e.g. `"broken_link"`) MUST surface as `Task.ErrorDetail` after `ListTasks`. **(fails first)**
- [ ] T003 Add `StatusExtra struct { ErrorDetail string \`json:"error_detail"\` } \`json:"status_extra"\`` to `dsmTask` and `ErrorDetail string \`json:"errorDetail"\`` to the flat `Task` (`server/internal/syno/client.go`); map it in `ListTasks` (`server/internal/syno/http.go`). Makes T002 green. (contracts/tasks-list.md)
- [ ] T004 [P] Add `ErrorDetail string \`json:"errorDetail"\`` to the mock `Task` and `/__mock/seed` shape and emit `status_extra.error_detail` from the task-list response when set (`server/internal/synomock/synomock.go`); extend `synomock_test.go` first to assert the emitted shape. (contracts/synomock-control.md)
- [ ] T005 [P] Client type: add `errorDetail?: string` to `Task` in `src/types/task.ts` (keep in lockstep with the server DTO).

**Checkpoint**: `errorDetail` flows server→wire→client and the mock can emit it. `go test ./...` + `npm run build` green.

---

## Phase 3: US2 — Understand why a download failed (P1) 🎯 MVP-critical, smallest slice

**Goal**: errored tasks show a human-readable reason.
**Independent test**: mock seeds an error task with `errorDetail`; the row shows a friendly reason (not just "Error").

- [ ] T006 [P] [US2] Write `src/services/task-error.test.ts`: known keywords → friendly text (broken_link, destination_not_exist, destination_denied, disk_full, torrent_duplicate, …), unknown/empty → generic "Download failed". **(fails first)**
- [ ] T007 [US2] Create pure module `src/services/task-error.ts` (`reasonFor(errorDetail: string): string`) per research.md D6; makes T006 green.
- [ ] T008 [US2] Add `src/services/task-error.ts` to the vitest coverage `include` allowlist in `vitest.config.ts` (floors ratchet up, never down).
- [ ] T009 [US2] Show the reason on errored rows in `src/components/TaskItem.vue` (only when `status === 'error'`), stock Ionic + `--app-*` tokens.
- [ ] T010 [US2] E2E `e2e/failure-reason.spec.ts`: seed an error task with `errorDetail` via `/__mock/seed`; assert the friendly reason is visible in the list.

**Checkpoint**: US2 shippable on its own.

---

## Phase 4: US1 — See downloads update live via SSE (P1) 🎯

**Goal**: the list/progress/speeds update on their own via the SSE stream.
**Independent test**: seed a downloading task; `/__mock/tick`; UI advances without a manual refresh.

- [ ] T011 [P] [US1] Write `server/internal/api/stream_limit_test.go`: the concurrent-stream limiter admits up to N and rejects beyond, releasing on close. **(fails first)**
- [ ] T012 [US1] Implement the global concurrent-stream bound in `server/internal/api/stream_limit.go` (mirrors `ratelimit`); makes T011 green.
- [ ] T013 [P] [US1] Write `server/internal/api/tasks_stream_test.go` (fake `syno.Client`): asserts `text/event-stream` headers; emits a `data:` snapshot; sends a heartbeat comment; returns `401`/terminal `session_expired` on the typed session-expired error; returns `503`+`Retry-After` over the cap; stops on request-context cancel. **(fails first)** (contracts/tasks-stream.md)
- [ ] T014 [US1] Implement `handleTasksStream` in `server/internal/api/tasks_stream.go` — stdlib `http.Flusher`; ~1s poll of the existing `syno.Client.ListTasks`; heartbeat `:\n\n` ~15s; per-connection sid via `requireSid`; `ctx.Done()` on disconnect; session-expiry mapping; wrap in the T012 limiter. Makes T013 green.
- [ ] T015 [US1] Register `mux.Handle("GET /v1/tasks/stream", ...)` in `server/internal/api/router.go`.
- [ ] T016 [P] [US1] Add `streamTasks()` to `src/services/api.ts`: `fetch` + `response.body.getReader()` + `TextDecoder`, `X-Syno-Sid` header (NOT `EventSource`); parse SSE `data:` lines into `{tasks, stats}`; expose an `AbortSignal` param and surface auth-error vs transport-error distinctly.
- [ ] T017 [US1] Rework `src/composables/useTasks.ts` to consume the stream while mounted AND `visibilityState==='visible'`; update reactive `tasks`/`stats` from each snapshot; `AbortController` on hide/unmount. (US4 adds fallback/backoff.)
- [ ] T018 [US1] E2E `e2e/live-updates.spec.ts`: seed a downloading task (`rate>0`), open the app, `POST /__mock/tick`, assert progress/speed advanced with NO manual refresh.

**Checkpoint**: live updates work while visible (fallback comes in US4).

---

## Phase 5: US4 — Stay working when the live connection drops (P2)

**Goal**: never a frozen list; auto-recover; survive idle >60s; session-expiry → login.

- [ ] T019 [US4] In `src/composables/useTasks.ts`: on stream open-failure / mid-stream error / close (non-auth), immediately `refresh()` + start the existing 3s poll, and retry the stream with capped backoff (1→2→5→10s); a successful reconnect stops the fallback poll; auth-error bypasses fallback → existing `SESSION_EXPIRED_EVENT` flow.
- [ ] T020 [P] [US4] E2E `e2e/live-fallback.spec.ts`: with the stream endpoint unavailable (or forced to error), the list still loads and updates via polling; assert no frozen/broken state.

**Checkpoint**: resilient live updates.

---

## Phase 6: US3 — See a task's full details (P2)

**Goal**: tap a row → stock-Ionic detail modal with the full field set + reason, live-updating.

- [ ] T021 [US3] Add an `@open` emit on row tap to `src/components/TaskItem.vue` (keep the existing sliding pause/resume/delete options).
- [ ] T022 [US3] Create `src/components/TaskDetailModal.vue` (stock `ion-modal`/`ion-list`/`ion-item`): full name, status, reason (when errored, via `task-error.ts`), destination, created time, size/downloaded/uploaded, peers/seeders, speeds; bound to the live task **by id** from the reactive collection; closes/greys if the id disappears.
- [ ] T023 [US3] Wire it in `src/views/tabs/TasksPage.vue`: open on `@open`, pass the id, live-update from `tasks`.
- [ ] T024 [US3] E2E `e2e/task-detail.spec.ts`: tap a task → modal shows the fuller fields; for an error task the reason is shown.

**Checkpoint**: all four stories complete.

---

## Phase 7: Polish & gates

- [ ] T025 Run all gates: `npm run build`; `npm run test:unit:coverage` (new `task-error.ts` floor holds); `cd server && go build ./... && go vet ./... && go test ./...` (syno floor ≥75, config ≥85); `npm run test:e2e`.
- [ ] T026 `make roadmap` (0002 → in-progress/in-review) and confirm the guard is satisfied.
- [ ] T027 Version bump for the release cycle (`npm run release:patch` → 0.0.3) so the merge cuts a release; commit subjects are user-facing "What's new" copy (`feat(tasks): downloads now update live`, `feat(tasks): failed downloads explain why`).
- [ ] T028 Open `feat/0002-live-task-updates` → `main` PR (only when the operator asks). After merge, verify Keel rolls k3s and live-check per quickstart.md (sid-safe).

---

## Dependencies & order

- **Phase 2 (foundational) blocks everything.** T003 depends on T002; T004/T005 are [P] with T002/T003 (different files).
- **US2 (Phase 3)** depends only on Phase 2 → smallest independent slice (recommended first).
- **US1 (Phase 4)** depends on Phase 2; independent of US2.
- **US4 (Phase 5)** depends on US1 (extends `useTasks`).
- **US3 (Phase 6)** depends on Phase 2 + `task-error.ts` (T007); independent of US1/US4 but nicer after them.
- Within a story: the `*_test` / `*.test` / `*.spec` task is written and failing BEFORE its implementation task.

## Parallel opportunities

- T002 / T004 / T005 (different files) after T001.
- T006 (US2 test) ‖ T011/T013 (US1 tests) — different files.
- T016 (client api) ‖ T014 (server handler) once contracts are fixed.

## MVP / incremental delivery

- **Smallest valuable slice**: Phase 2 + US2 (failure reason) — directly fixes what the operator sees, tiny surface.
- **Core feature**: add US1 (live SSE) + US4 (fallback).
- **Complete**: add US3 (detail view). Ship all four as 0.0.3.
