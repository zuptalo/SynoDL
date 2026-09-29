# Tasks: Admins run the music library repair from Settings

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: REQUIRED (constitution II). Within every phase the test tasks come first and MUST fail before the implementation that satisfies them. Gates: `scripts/music-repair-test.sh` (tool), `cd server && go build ./... && go vet ./... && go test ./...`, `npm run build`, `npm run test:unit:coverage`, `npm run test:e2e` (locally with `CHROMIUM_PATH`).

**Issues**: #313 Gate + tool events · #314 Foundations · #315 US1 · #316 US2 · #317 US3 · #318 US4 · #319 US5 · #320 Delivery (the PR into `main` lists `Closes #313` … `Closes #320`).

**Format**: `- [ ] T### [P?] [US?] description with file path` — `[P]` = different files, no dependency on an incomplete task.

## Phase 0: Gate before code

- [x] T000 Run `/speckit-analyze`, then `/speckit-checklist` (security: worker orchestration, worker-output parsing, admin authorisation, one-at-a-time), fix every gap in spec/plan/tasks, then `/speckit-taskstoissues`

## Phase 1: The tool reports (Python; blocks everything that reads a run)

- [ ] T001 [P] Write failing `scripts/music_repair/test_events.py`: `events.emit` writes exactly one `@@synodl {json}` line, ≤ 4096 bytes, no embedded newline, `ensure_ascii=False`; `progress` shape; `result` shapes for check / apply / undo / refusal per `contracts/worker-events.md`; caps (12 reasons, 20 check examples, 10 apply/undo examples, path 200, reason 120); control characters stripped; a hostile path or reason cannot produce a second line or a second event
- [ ] T002 Implement `scripts/music_repair/events.py` (`emit_progress`, `emit_result`, `summarise_plan(plan, free_bytes)`, `summarise_apply(res)`, `summarise_restore(res)`) until T001 passes
- [ ] T003 Write failing additions to `scripts/music_repair/test_cli.py`: `plan` prints progress events at `scan`, `lookup` (every 25), `plan` and ends with exactly one `result` (kind check) whose numbers equal the plan's totals; `apply` prints `apply` progress and a `result` whose counts equal the journal's; `restore` likewise; refusals (`locked`, `no_plan`, `no_space`, `rejected`) end with an `ok:false` result and the existing exit code 4; the result is the LAST event; the human lines are unchanged; no video description, URL or environment value appears in any event
- [ ] T004 Wire the events into `scripts/music_repair/__main__.py` (and the phase boundaries in `planner.py`/`applier.py` via a callback, not by importing events into pure modules) until T003 passes

**Checkpoint**: the operator tool is unchanged for operators and now speaks a fixed, bounded protocol.

## Phase 2: Foundations (server plumbing; pure logic first)

- [ ] T005 [P] Write failing `server/internal/k8s/pods_test.go` additions and `jobs_test.go` additions: `GetPod` returns `imageID` and the container image from an `httptest` fake, 404/403 map through `IsNotFound`/`IsForbidden`, body bounded; `Job` JSON carries `initContainers`, `volumeMounts[].readOnly`, and no field that was not asked for
- [ ] T006 Implement `server/internal/k8s/types.go` (`InitContainers`, `VolumeMount.ReadOnly`, Pod `Spec.Containers[].Image`, `Status.ContainerStatuses[]{Name,Image,ImageID}`) and `pods.go` (`GetPod`) until T005 passes
- [ ] T007 [P] Write failing `server/internal/musicrepair/events_test.go`: the parser table from `contracts/worker-events.md` — valid progress/result; non-prefixed lines ignored; line > 4 KiB dropped; malformed JSON dropped; unknown event/phase/reason dropped; integers clamped to `[0,10¹²]`; negative → 0; control characters stripped; strings capped; arrays capped; `planId` pattern enforced; last valid `result` and last valid `progress` win; at most 64 event lines considered; empty input → no result; a result that appears BEFORE later garbage is still found
- [ ] T008 Implement `server/internal/musicrepair/events.go` (`ParseLog([]byte) (Progress *Progress, Result *Summary)`) until T007 passes
- [ ] T009 [P] Write failing `server/internal/musicrepair/job_test.go`: `BuildJob` — init container runs the SELF image with `cp -r /opt/music_repair /code/` and mounts only the `emptyDir`; main container is the pinned worker image (never `:latest`, never empty); exactly ONE PVC volume, the music claim; `/code` read-only in the main container; `automountServiceAccountToken:false`; `runAsUser/Group` from config; labels `managed-by=synodl`, `kind=music-repair`, `repair-id`, and `app.kubernetes.io/name=synodl-music-repair`; the download selector (`kind=ytdl`) does NOT match; args per kind are separate argv elements; an invalid plan id is refused before any Job is built; deadline 12 h; no env value from a request
- [ ] T010 Implement `server/internal/musicrepair/job.go` (`BuildJob`, `Selector`, `AnySelector`, `PlanIDPattern`) until T009 passes
- [ ] T011 [P] Write failing `server/internal/musicrepair/state_test.go`: run state from (record, Job, result): running / finished / refused / unfinished / vanished-after-grace; plan status and `canApply` / `canContinue` / `canUndo` for every case in `research.md` R5 including the 24 h boundary (23h59m yes, 24h01m no), applied, apply-unfinished, undone, a check with no plan id, and "a run is active"
- [ ] T012 Implement `server/internal/musicrepair/state.go` until T011 passes
- [ ] T013 [P] Write failing `server/internal/store/music_repairs_test.go`: insert `running` succeeds once and a second concurrent insert fails (run with N goroutines: exactly one wins); `Finish` sets state/summary/finished_at once; list newest-first; prune keeps the newest 50 and never the running one; deleting the user keeps the row with its `user_name`; the migration is idempotent (replayed by the drift repair)
- [ ] T014 Implement migration 0040 in `server/internal/store/schema.go` and `server/internal/store/music_repairs.go` until T013 passes

**Checkpoint**: parsing, Job assembly, state rules and storage exist and are proven, with no HTTP yet.

## Phase 3: User Story 1 — Check the library and read what would change (P1) 🎯 MVP

**Goal**: an admin starts a check, watches it, reads the summary; nothing on the share changes.
**Independent test**: e2e against the mock cluster — start, progress, result; non-admin has no section; unavailable is explained.

- [ ] T015 [US1] Write failing `server/internal/api/music_repair_handlers_test.go`: `GET /v1/library/repair` — 403 for non-admin and for anonymous; `available:false` with `reason` for each of no cluster (`Jobs == nil`), no music claim, no image; empty snapshot when nothing has run; `current` with `progress` when running; `POST …/check` — 202 and exactly one Job with the right labels; 409 `busy` with `startedBy`/`startedAt` when a run is active; 409 `busy` with `source:"command_line"` when a Job labelled only `app.kubernetes.io/name=synodl-music-repair` is active; when `CreateJob` fails the slot is released and the answer is 502 `start_failed`; two simultaneous requests → exactly one Job; no response body ever contains worker output (FR-016)
- [ ] T016 [US1] Implement `server/internal/api/music_repair_handlers.go` (snapshot, check), the self-image resolver (`MUSIC_REPAIR_IMAGE` override, else `GetPod($HOSTNAME)` → `imageID`, cached in a pointer on `Deps`), config field + `Load()` (`MusicRepairImage`), and route registration inside the `Stateful` block in `router.go`, until T015 passes
- [ ] T017 [US1] Write failing `server/internal/api/music_repair_reconcile_test.go`: on a tick, a `running` record whose Job Completed with a valid result → `finished` with the decoded summary (or `refused` when `ok:false`); Job Failed / deadline → `unfinished`; Job absent beyond the 60 s grace → `unfinished`, within it → still running; malformed or missing report on a Completed Job → `finished` with empty summary; the outcome is captured exactly once (re-tick is a no-op); a NEW `Deps` over the same store and the same Jobs (a server restart) picks a running record back up with its true state (SC-004); a record whose Job was swept while the server was down becomes `unfinished` with no summary (outcome unknown), never `finished`; ytdl Jobs and unrelated Jobs are never touched or deleted; worker output never reaches the server log (capture `slog` output)
- [ ] T018 [US1] Implement `server/internal/api/music_repair_reconcile.go` and call it from the existing download reconciler's tick (one loop), and progress reading (bounded tail, cached for the poll interval) used by `GET`, until T017 passes
- [ ] T019 [P] [US1] Write failing `src/services/repair-state.test.ts`: pure helpers — what to offer for each snapshot (unavailable / idle / running / result), stage wording, percent, `expiresIn` wording, byte formatting, headline for history rows, "left alone" grouping
- [ ] T020 [US1] Implement `src/services/repair-state.ts` until T019 passes; add it to the vitest coverage allowlist (`vitest.config.*`) with its floor
- [ ] T021 [US1] Add repair types and calls to `src/services/api.ts` (`getMusicRepair`, `startMusicRepairCheck`); build `src/components/MusicRepairModal.vue` (unavailable, idle with *Check library*, running with `ion-progress-bar` and stage, result list with the numbers and the "left alone" accordion, where the plan file is) and the admin entry in `src/views/tabs/SettingsPage.vue` (`data-testid` on every control); poll every 3 s only while open and a run is active
- [ ] T022 [US1] Write and pass `e2e/stateful/music-repair.spec.ts` (check flow): admin starts a check, the mock Job is driven with `/__mock/jobs/{name}/emit` progress then a result event and `/succeed`, the screen shows the same numbers, closing and reopening keeps state, a second start is refused with who/when, a non-admin has no entry and gets 403, and with no music library configured the section explains why and nothing else changes; at a 360 px viewport the modal has no horizontal scroll, and it renders in the dark theme (SC-009, FR-021)

**Checkpoint**: MVP — an admin can audit the library from Settings without kubectl.

## Phase 4: User Story 2 — Apply a plan the admin has just reviewed (P1)

**Goal**: apply exactly one reviewed, fresh plan, behind the snapshot acknowledgement.
**Independent test**: refused without a check / expired / applied / without the acknowledgement; succeeds otherwise, with progress and an outcome.

- [ ] T023 [US2] Write failing apply tests in `music_repair_handlers_test.go`: 400 `snapshot_required` without `snapshotAck:true` (and the Job is NOT created); 400 for a `planId` that does not match the pattern (never reaches argv); 409 `no_plan` for an unknown plan (including one made from the command line); 409 `plan_expired` at 24 h + 1 s and accepted at 24 h − 1 s; 409 `already_applied`; 409 `busy`; success creates one Job with the plan id as its own argv element; an apply left `unfinished` can be continued (same plan id, new run row); the response and the recorded row never contain the snapshot text or worker output
- [ ] T024 [US2] Implement apply in `music_repair_handlers.go` until T023 passes
- [ ] T025 [US2] Extend `MusicRepairModal.vue`: *Apply this plan* opens a confirmation sheet listing what will change, that nothing is deleted, that it can be undone, and an `ion-checkbox` "I have taken a snapshot of the music share" that gates the confirm `ion-button` (un-ticking disables it again; the acknowledgement is never remembered); progress and outcome (done / skipped with reasons / failed); *Continue* for an unfinished apply; add the calls to `api.ts`
- [ ] T026 [US2] Extend the e2e spec: apply is not offered without a fresh check; the confirm button stays disabled until the box is ticked; the apply runs and shows its outcome; an unfinished apply offers *Continue* (plan expiry is proven in the Go tests T023 — a real server binary has no clock seam to age a plan in e2e)

## Phase 5: User Story 3 — Undo the last applied repair (P2)

- [ ] T027 [P] [US3] Write failing undo tests in `music_repair_handlers_test.go`: 409 `not_undoable` for never-applied / already-undone / while a run is active; success creates a `restore` Job with the plan id; the outcome records restored/skipped
- [ ] T028 [US3] Implement undo in `music_repair_handlers.go` until T027 passes
- [ ] T029 [US3] Add *Undo* (with its own confirmation) to `MusicRepairModal.vue` and `api.ts`; extend `repair-state` tests/impl for undo availability
- [ ] T030 [US3] Extend the e2e spec: apply then undo; undo is not offered for a repair that was never applied or was already undone

## Phase 6: User Story 4 — History (P2)

- [ ] T031 [P] [US4] Write failing tests: history is newest first, bounded to 50, shows kind / starter / times / state / headline, keeps a deleted user's name, and survives a "restart" (a new `Deps` over the same store)
- [ ] T032 [US4] Implement history in the snapshot and add the history list to `MusicRepairModal.vue` until T031 and `repair-state` tests pass
- [ ] T033 [US4] Extend the e2e spec: run two operations as two admins; the history lists each with the right person and outcome

## Phase 7: User Story 5 — Honest when it cannot run (P3)

- [ ] T034 [US5] Write and pass a Go test that with `Jobs == nil` and with no music claim every non-repair route behaves exactly as before and the repair snapshot answers `available:false`; add the same to e2e for the stateless stack (the section is absent)

## Phase 8: Delivery, docs and gates

- [ ] T035 [P] `Dockerfile`: (the init container runs as the worker's unprivileged uid, so the copied files must be world-readable — assert that in the image check) runtime stage `COPY scripts/music_repair/ /opt/music_repair/` (verify `.dockerignore` keeps it); build the image locally and prove `docker run --rm --entrypoint sh <img> -c 'ls /opt/music_repair && cp -r /opt/music_repair /tmp/c && ls /tmp/c'`; confirm `deploy/k8s/30-rbac.yaml` is byte-identical to `main` (no RBAC change) and record that in the spec
- [ ] T036 [P] Config and dev wiring: `MUSIC_REPAIR_IMAGE` in `server/internal/config`, the `Makefile` dev env, the e2e stateful stack env, `deploy/k8s/10-synodl.yaml` comment (unset in production: the server reads its own pod); `HOSTNAME` is already the pod name
- [ ] T037 [P] Docs: `docs/MUSIC-LIBRARY-REPAIR.md` ("From Settings"), `CLAUDE.md` pointer, `deploy/k8s/README.md`, and the spec's Verification section
- [ ] T038 Security review test: over a full e2e run, no worker output, plan path or example path appears in the server's log output; the JSON of every repair response is fixed-shape (no field carries raw log text); the RBAC file and the Role verbs are unchanged
- [ ] T039 Gates: `scripts/music-repair-test.sh`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run build`, `npm run test:unit:coverage`, `npm run test:e2e`
- [ ] T040 Commit in logical chunks (Conventional Commits; the user-facing one reads as release-note copy), push, set `**Status**: in-review`, `make roadmap`, open the PR (`Closes #N` for each issue) and merge when green

## Dependencies

- Phase 0 → all. Phase 1 (tool) blocks the server tests that use its real output, but Phase 2 can start in parallel (the parser is specified by the contract, not by the tool).
- Phase 2 → Phases 3–7. US1 (T015–T022) is the MVP. US2 needs US1's handlers and modal. US3 needs US2 (something applied). US4 needs the store (Phase 2) and the modal. US5 is verification.
- T035–T037 are independent of each other and of the user stories; T038–T040 close the work.

## Parallel opportunities

- Phase 2: T005, T007, T009, T011, T013 (five test files) in parallel; then their implementations.
- US1: T019 (client pure module) ∥ T015/T017 (server tests).
- Phase 8: T035 ∥ T036 ∥ T037.

## Implementation strategy

MVP = Phases 1–3: an admin can audit the library from Settings and the server proves it
learns of a run only through a bounded, parsed report. Then US2 (the risky half, so its
guards are written as tests first), US3, US4, US5. Each story is releasable on its own.
