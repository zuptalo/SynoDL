# Tasks: Connect to Download Station, view tasks, and add downloads

**Input**: spec.md + plan.md in this directory.
**Tests**: REQUIRED — constitution Principle II mandates TDD (failing tests
before the implementation that satisfies them, Red → Green → Refactor). Every
RED task must fail before its GREEN counterpart lands. New user-facing behavior
lands an e2e spec per story.

**Format**: `- [ ] [ID] [P?] [Story] Description with file path`

## Phase 1: Setup

- [ ] T001 Extend the vitest coverage allowlist in `vitest.config.ts` with
      `src/services/task-sort.ts`, `src/services/url-detect.ts`,
      `src/services/syno-errors.ts` (ratchet: add, never remove).

## Phase 2: Foundational (blocking prerequisites)

*(The proxy, mock DSM, session gate, polling list, and e2e harness shipped
with the scaffold — no foundational work remains.)*

## Phase 3: User Story 1 — Connect to my NAS (P1)

**Independent test**: sign in against the mock (plain + OTP accounts), reload
persistence, distinct auth errors, logout.

- [ ] T002 [US1] RED: `src/services/syno-errors.test.ts` — every ApiError code
      (`credentials`, `otp_required`, `otp_invalid`, `permission`,
      `nas_unreachable`, `session`, unknown) maps to a distinct plain-language
      message; unknown codes get a generic fallback.
- [ ] T003 [US1] GREEN: implement `src/services/syno-errors.ts` until T002
      passes; refactor `LoginPage.vue` to use it (drop the inline map).
- [ ] T004 [US1] RED→GREEN: `e2e/connect.spec.ts` — wrong-password error,
      OTP-required flow (`otpuser`: field appears, wrong code distinct error,
      `000000` succeeds), session survives reload, expired-sid bounce
      (reset mock sessions, next poll returns to /login), logout forgets.

**Checkpoint**: US1 independently shippable.

## Phase 4: User Story 2 — See my downloads live (P1)

**Independent test**: seeded fixtures render every field; state changes appear
within one poll.

- [ ] T005 [US2] RED→GREEN: `e2e/tasks-list.spec.ts` — field-level assertions
      on seeded fixtures (name, status chip, percent+size line, speeds, ETA),
      global header speeds, empty state, live update after a mock pause/seed
      change within one poll interval, zero-size magnet renders 0% safely.

**Checkpoint**: US1+US2 = smallest shippable product.

## Phase 5: User Story 3 — Add a download (P2)

- [ ] T006 [P] [US3] RED: `src/services/url-detect.test.ts` — extracts
      http/https/ftp/ftps/magnet/thunder URLs from multi-line/space-separated
      text, ignores junk, dedupes, preserves order; empty input → [].
- [ ] T007 [US3] GREEN: implement `src/services/url-detect.ts` until T006
      passes.
- [ ] T008 [US3] Build `src/components/FolderPickerModal.vue` — shares →
      subfolder drill-down over `/v1/fs`, breadcrumb back navigation, radio
      selection; returns the picked share-relative destination.
- [ ] T009 [US3] Build `src/components/NewTaskModal.vue` — URL textarea with
      live "N links detected" count (url-detect), clipboard paste button
      (graceful permission fallback), `.torrent` file input, destination row
      opening T008, optional source credentials + extract password fields,
      confirm disabled until a URL or file is present; FAB on TasksPage opens
      it.
- [ ] T010 [US3] RED→GREEN: `e2e/add-task.spec.ts` — multi-URL create lands
      one task per URL with picked destination, torrent upload appears with
      the file's name, junk-only input keeps confirm disabled, oversized
      torrent (mock a >16 MiB file) surfaces the clear 413 message.

**Checkpoint**: US3 independently shippable.

## Phase 6: User Story 4 — Control tasks (P2)

- [ ] T011 [US4] Extend `src/components/TaskItem.vue` with ion-item-sliding:
      pause/resume (state-appropriate) and delete behind an ion-action-sheet
      confirmation; wire `/v1/tasks/pause|resume|delete` with an optimistic
      refresh.
- [ ] T012 [US4] RED→GREEN: `e2e/task-actions.spec.ts` — pause a downloading
      fixture (status flips), resume it back, delete with confirm removes it,
      cancel keeps it.

**Checkpoint**: US4 independently shippable.

## Phase 7: User Story 5 — Filter and sort (P3)

- [ ] T013 [P] [US5] RED: `src/services/task-sort.test.ts` — each sort key
      (createdAt, status, size, peers, downloadSpeed, uploadSpeed, name,
      ratio=uploaded/downloaded, progress, remaining) both directions with
      stable tie-break by id; case-insensitive term filter; status
      multi-select; zero-size/zero-speed edge cases (remaining=∞ sorts last).
- [ ] T014 [US5] GREEN: implement `src/services/task-sort.ts` until T013
      passes.
- [ ] T015 [US5] Build `src/composables/useTaskFilter.ts` — filter state
      persisted as the `taskFilter` row of the idb `settings` store (defaults:
      createdAt desc, all statuses on, empty term).
- [ ] T016 [US5] Build `src/components/TaskFilterSheet.vue` — term input, sort
      radio list + asc/desc, status checkboxes, Apply commits to T015; filter
      button on TasksPage opens it; TasksPage renders through task-sort.
- [ ] T017 [US5] RED→GREEN: `e2e/filter-sort.spec.ts` — size sort both
      directions, term narrows, unchecked status hides, choices survive a
      reload.

## Phase 8: Polish & gates

- [ ] T018 Checklist pass: complete `checklists/credential-safety.md`
      (Principle III — spec touches login credentials + source credentials).
- [ ] T019 Full gate: `npm run build`, `npm run test:unit:coverage` (new
      floors green), `cd server && go build ./... && go vet ./... && go test
      ./...`, `npm run test:e2e` (all six e2e specs).
- [ ] T020 Bump spec Status → `in-review`, run `make roadmap`, commit the
      regenerated `ROADMAP.md`.

## Dependencies & execution order

- T001 first (allowlist ready before RED tasks).
- Within each story: RED before GREEN before UI wiring (Principles II).
- Stories in priority order US1→US5; T006/T013 are [P] (pure, parallel-safe).
- T018–T020 last.
