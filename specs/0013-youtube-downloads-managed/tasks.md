---
description: "Task list for spec 0013 — YouTube downloads you can watch, keep, and retry"
---

# Tasks: YouTube downloads you can watch, keep, and retry

**Input**: Design documents from `/specs/0013-youtube-downloads-managed/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/http-api.md

**Count**: 114 tasks, 42 of them tests. Seven were added by the `/speckit-analyze`
gate (T061a, T061b, T066a, T079a, T079b, T100a, T105a) to close coverage gaps — the
lettered ids keep execution order readable without renumbering.

**Tests**: REQUIRED, not optional. Constitution Principle II mandates that failing
tests are ordered before the implementation that satisfies them (Red → Green →
Refactor), that new handler logic ships unit tests, and that new user-facing
behaviour extends `e2e/`. Every `[TEST]` task below must fail before its
implementation task is started.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable — different files, no dependency on an incomplete task
- **[Story]**: US1–US8 from spec.md; Setup / Foundational / Polish carry none

## Path Conventions

Monorepo: client at repository root (`src/`, `e2e/`), Go service under `server/`.

---

## Phase 1: Setup

**Purpose**: Establish the constitutional basis and the operator knob. Nothing
below may start before T001 — Principle I makes code without an approved
constitutional basis a defect.

- [X] T001 Amend `.specify/memory/constitution.md` 2.1.0 → 2.2.0: Principle III gains (a) durable pre-admission work is not a mirror because no worker exists, (b) a durable record of requested and finished work including successes, (c) reading a worker's own output as a permitted least-privilege capability that still excludes secrets, exec and attach. Add the Sync Impact Report entry at the top in the existing style.
- [X] T002 [P] Add `YtdlMaxParallel` to `server/internal/config/config.go` reading `YTDL_MAX_PARALLEL`, defaulting to 4, and document it beside the other `Ytdl*` fields.
- [X] T003 [P] [TEST] Add a config case in `server/internal/config/config_test.go` covering the default, an operator override, and a non-numeric value falling back to the default.
- [X] T004 [P] Add `YTDL_MAX_PARALLEL` to `deploy/k8s/10-synodl.yaml` and to the `Makefile` dev defaults so `make start` exercises the same knob.

---

## Phase 2: Foundational

**Purpose**: The shared machinery more than one story needs and no single story
owns — reading worker output, and the loop that does everything periodic.

**Note**: US1 and US2 do NOT depend on this phase and may ship before it.

### Reading a worker's output

- [X] T005 [P] [TEST] Add `server/internal/k8s/pods_test.go` covering `ListPods` by selector and `PodLog` against an `httptest` fake API server: text/plain response, `tailLines` and `container` passed through, a 404 surfacing as `IsNotFound`, and a body larger than the cap being truncated rather than read whole.
- [X] T006 Add `Pod` and `PodList` to `server/internal/k8s/types.go` as hand-written subsets carrying only the fields SynoDL reads.
- [X] T007 Add `ListPods(ctx, selector)` and `PodLog(ctx, name, opts)` to `server/internal/k8s/jobs.go`. `PodLog` returns bytes, not JSON — `do()` decodes JSON and cannot be reused as-is. Bound the read (FR-013e).
- [X] T008 Update the package doc in `server/internal/k8s/types.go`: it no longer describes "Jobs in ONE namespace, and nothing else". Say what the two pod calls are for and why they need no new client dependency.
- [X] T009 Add `pods/log` with verb `get` to the Role in `deploy/k8s/30-rbac.yaml`, and extend the existing "note what is deliberately ABSENT" comment to record that reading a worker's output is not the same permission as controlling one.
- [X] T010 Extend `JobRunner` in `server/internal/api/ytdl_handlers.go` with the two pod calls, and update the fake in `server/internal/api/fake_test.go`.

### The mock orchestrator

- [X] T011 [P] Add pod listing and `GET /api/v1/namespaces/{ns}/pods/{name}/log` to `server/internal/k8smock/k8smock.go`, so the wire format and the selector are exercised rather than faked at the Go boundary.
- [X] T012 [P] Add `/__mock/jobs/{name}/emit` to `server/internal/k8smock/k8smock.go` to append arbitrary lines to a job's pod log, so tests drive progress, lyrics and expansion output deterministically.
- [X] T013 [P] [TEST] Extend `server/internal/k8smock/k8smock_test.go` for pod listing, log reading, and the emit control.

### The reconciler

- [X] T014 [TEST] Add `server/internal/api/ytdl_reconcile_test.go` asserting the loop runs on its ticker, is driven by running downloads rather than by request handlers (FR-013f), and survives a `ListJobs` error without exiting.
- [X] T015 Create `server/internal/api/ytdl_reconcile.go` with the ticker skeleton — list this feature's Jobs once per tick, and expose seams for the per-story responsibilities added in later phases. Follow the `library_scan.go` / `source_keepalive.go` shape.
- [X] T016 Start the reconciler from `server/cmd/synodl/main.go`, guarded so a deployment without an orchestrator never starts it.

**Checkpoint**: worker output is readable, the mock can produce it, and one loop exists to consume it.

---

## Phase 3: User Story 1 — My downloads are mine (P1)

**Goal**: Close the live defect — every signed-in user currently sees every
user's YouTube downloads.

**Independent test**: Submit downloads as two users; each sees only their own; an
admin sees both with attribution.

**Depends on**: T001 only.

- [X] T017 [P] [US1] [TEST] Add ownership cases to `server/internal/api/ytdl_handlers_test.go`: the list returns only the caller's rows; an admin gets all rows with `submittedBy`; a stored failure with a NULL `user_id` is admin-visible only.
- [X] T018 [P] [US1] [TEST] In `server/internal/api/ytdl_handlers_test.go`, add cases asserting dismiss returns **404, not 403**, for another user's download, and that a non-existent id is indistinguishable from it (FR-008).
- [X] T019 [US1] Filter `handleYtdlList` in `server/internal/api/ytdl_handlers.go` by the caller's id, reading the existing `AnnSubmittedBy` annotation and `ytdl_failures.user_id`, with the admin exception stated positively (FR-009a).
- [X] T020 [US1] Gate `handleYtdlDismiss` on ownership in `server/internal/api/ytdl_handlers.go`, returning 404 for a download the caller may not see.
- [X] T021 [US1] Add a `ListYtdlFailuresForUser` query to `server/internal/store/ytdl_repos.go` so filtering happens in SQL rather than after loading every row.
- [X] T022 [P] [US1] [TEST] Add `server/internal/store/ytdl_repos_test.go` cases for the per-user and admin queries, including the NULL-`user_id` case.
- [X] T023 [US1] ~~Require a session on~~ **Pin the disclosure property of** `handleYtdlThumb` in `server/internal/api/ytdl_thumb.go` (FR-009d). Revisited during implementation: gating it was the wrong answer — the caller supplies the address, the content is public, and `<img src>` carries no session, so a check would cost caching or leak a token into URLs to protect nothing. The host allowlist is the control; `ytdl_thumb_test.go` now asserts the endpoint discloses nothing about the instance's downloads.
- [X] T024 [US1] Add `e2e/stateful/ytdl-ownership.spec.ts`: two users, each seeing only their own download, and an admin seeing both.

**Checkpoint**: US1 is independently shippable.

---

## Phase 4: User Story 2 — What I downloaded is still there next week (P1)

**Goal**: Every download gets a durable record that outlives the worker.

**Independent test**: Submit, let it finish, remove the Job, confirm the download
is still listed with title, artwork, link and both timestamps.

**Depends on**: Phase 1. Uses Phase 2's reconciler for capture (T033).

- [ ] T025 [US2] [TEST] Add `server/internal/store/ytdl_repos_test.go` cases for the `ytdl_downloads` table: insert, read back, page by cursor, the `(video_id, mode)` already-held lookup, group aggregate counts, and that a deleted user leaves the row with a NULL `user_id` rather than deleting it.
- [ ] T026 [US2] [TEST] Add a case in `server/internal/store/migrations_golden_test.go` asserting the new migration runs twice without error — the spec 1031 drift repair rewinds and replays, so a migration that cannot run twice is a boot failure. Add a second case for FR-006c: a migration that fails part-way leaves the existing data intact and the failure reportable, rather than starting against a half-changed store.
- [ ] T027 [US2] Append the migration to `server/internal/store/schema.go` creating `ytdl_downloads` per `data-model.md`, every statement `IF NOT EXISTS`, with the backfill from `ytdl_failures` as `INSERT OR IGNORE`. `user_id` is `ON DELETE SET NULL`, matching the existing convention. Do not drop `ytdl_failures`.
- [ ] T028 [US2] Add the `Download` type and its repository functions to `server/internal/store/ytdl_repos.go`: create, get, list-by-user paged, list-by-parent, update-state, already-held, delete-with-children.
- [ ] T029 [US2] [TEST] Add cases to `server/internal/api/ytdl_handlers_test.go` asserting submit creates a durable record before anything else, and that the list merges live Jobs with stored records without duplicating a request id.
- [ ] T030 [US2] Change `handleYtdlSubmit` in `server/internal/api/ytdl_handlers.go` to persist the record first and return `201` with the record's state, per `contracts/http-api.md`.
- [ ] T031 [US2] In `server/internal/api/ytdl_handlers.go`, change `handleYtdlList` to read from `ytdl_downloads`, paged by cursor (FR-006a), returning `nextCursor`.
- [ ] T032 [US2] [TEST] In `server/internal/api/ytdl_handlers_test.go`, add a case asserting a record that cannot be written never causes an already-saved download to be reported as failed (FR-006b).
- [ ] T033 [US2] Add terminal-fact capture to `server/internal/api/ytdl_reconcile.go`: when a download reaches a final state, write outcome, reason and `finished_at` durably **while the worker's output is still readable** (FR-013g).
- [ ] T034 [US2] [TEST] Extend `e2e/stateful/ytdl.spec.ts` to delete the Job via `/__mock/*` and assert the download is still listed with its title, artwork and link.

**Checkpoint**: history survives the sweep; US1 + US2 are a coherent shippable increment.

---

## Phase 5: User Story 3 — Watch a download actually progress (P1)

**Goal**: A real progress bar, and lyrics/language reported.

**Independent test**: Drive a worker that emits progress; the row advances and
then reaches its final state; the finished download reports lyrics and language.

**Depends on**: Phase 2, Phase 4.

- [ ] T035 [P] [US3] [TEST] Add `server/internal/ytdl/progress_test.go` — table-driven: a well-formed sentinel line parses; a line without the sentinel is ignored, never guessed at; malformed numbers are ignored; two passes of a two-stream fetch never produce a decreasing value (FR-012); a stale reading is dropped rather than shown (FR-013).
- [ ] T036 [US3] Create `server/internal/ytdl/progress.go`: the `--progress-template` constant with its fixed sentinel prefix, the line parser, and the monotonic clamp. Pure — no I/O.
- [ ] T037 [P] [US3] [TEST] Add cases to `server/internal/ytdl/progress_test.go` for detecting the companion-file line and extracting its language.
- [ ] T038 [US3] Add companion-file and language detection to `server/internal/ytdl/progress.go`.
- [ ] T039 [US3] [TEST] Add cases to `server/internal/ytdl/command_test.go` asserting `--newline` and the progress template are present, that the template is a constant containing no user input, and that the URL is still the final argv element exactly once after `--`.
- [ ] T040 [US3] Add `--newline` and `--progress-template` to `Args` in `server/internal/ytdl/command.go`.
- [ ] T041 [US3] Add output reading to `server/internal/api/ytdl_reconcile.go`: for each running download, read its worker's log bounded in size and frequency (FR-013e/f), hold the reading in memory, and capture the companion-file facts durably.
- [ ] T042 [US3] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add cases asserting an unreadable or unparseable log leaves the download's life state correct and shows no percentage, and never reports it as failed (FR-013).
- [ ] T043 [US3] Add `progress`, `hasLyrics` and `lyricsLang` to the wire view in `server/internal/api/ytdl_handlers.go`, omitted rather than zero-valued when absent.
- [ ] T044 [P] [US3] Add a progress bar to `src/components/YtdlItem.vue`, shown only in the `downloading` state, matching `TaskItem.vue` row metrics.
- [ ] T045 [P] [US3] Update `YtdlDownload` in `src/services/api.ts` and `src/composables/useYtdl.ts` for the new fields.
- [ ] T046 [US3] Add `e2e/stateful/ytdl-progress.spec.ts` driving progress lines through `/__mock/jobs/{name}/emit` and asserting the bar advances and never goes backwards.

**Checkpoint**: the most-felt gap in spec 0012 is closed.

---

## Phase 6: User Story 4 — Open one and see everything about it (P2)

**Goal**: A detail view with every gathered fact, in the fixed date format.

**Independent test**: Open a finished download and a failed one; every listed
fact is present and correctly formatted.

**Depends on**: Phase 4, Phase 5.

- [ ] T047 [P] [US4] [TEST] Add `src/utils/format.spec.ts` cases for a fixed `yyyy-mm-dd` / `HH:MM:SS` formatter, asserting it does NOT follow device locale (FR-031) — run it under at least two locales.
- [ ] T048 [P] [US4] Add the fixed formatter to `src/utils/format.ts`. Do not reuse the existing `toLocaleString(undefined, …)` path, which is locale-following by design.
- [ ] T049 [US4] [TEST] Add `server/internal/api/ytdl_detail_test.go` for `GET /v1/ytdl/{requestId}`: every field per the contract; `finishedAt` omitted rather than zero while unfinished (FR-033); `submittedBy` for admins only; 404 for another user's download.
- [ ] T050 [US4] Create `server/internal/api/ytdl_detail.go` with the detail handler, and register the route in `server/internal/api/router.go`.
- [ ] T051 [P] [US4] Create `src/components/YtdlDetailModal.vue` following the `TaskDetailModal.vue` convention — a stock-Ionic sheet with `data-testid="ytdl-detail"` and `ytdl-detail-*` field ids, including the playlist or channel an expanded item came from (FR-019c), which is only visible here when an item is viewed on its own. Not a route; the router has no per-item routes.
- [ ] T052 [US4] Open the detail modal from `src/components/YtdlItem.vue` on tap.
- [ ] T053 [US4] [TEST] In `server/internal/api/ytdl_detail_test.go`, add a case asserting a failure reason contains no command line, path or raw worker output (FR-032).
- [ ] T054 [US4] Add `e2e/stateful/ytdl-detail.spec.ts` asserting the full field set and both timestamp formats.

---

## Phase 7: User Story 7 — Four at a time, the rest wait their turn (P2)

**Goal**: An instance-wide limit of four, shared fairly, durable across restart.

**Independent test**: Queue more than four; exactly four run; each finish starts
the next; a restart resumes rather than losing the queue.

**Depends on**: Phase 2, Phase 4.

- [ ] T055 [P] [US7] [TEST] Add `server/internal/ytdl/queue_test.go` — table-driven over the fair-share ordering: one user's large group never starves another's single link (FR-022b); a direct submission outranks the same user's expanded items (FR-022c); with only one user having work, no slot idles; submission order is the tiebreak. Assert SC-005a directly: with one user's channel occupying the queue, a second user's link is next to be admitted, so it waits for at most one running download.
- [ ] T056 [US7] Create `server/internal/ytdl/queue.go` with the admission ordering as a pure function over the queued set. No I/O, no clock.
- [ ] T057 [US7] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add cases asserting never more than the limit run, that a finish in either terminal state admits the next, and that the operator's limit is what is enforced.
- [ ] T058 [US7] Add admission to `server/internal/api/ytdl_reconcile.go`: count running, take that many from the ordering, create their Jobs. Single replica means one admitter and no distributed lock — record that in a comment.
- [ ] T059 [US7] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add a case asserting a restart with work in flight resumes the queue and double-admits nothing (FR-023).
- [ ] T060 [US7] Surface `queued` distinctly from `scheduled` in `src/components/YtdlItem.vue` (FR-013b) — "waiting its turn" must not read as "starting now".
- [ ] T061 [US7] [TEST] In `server/internal/api/ytdl_handlers_test.go`, add cases asserting dismissing a queued download removes it from the queue so it never starts, and dismissing a running one does not strand its worker (FR-005b, FR-005c). Add a case for FR-009b: a group may be dismissed only by its owner or an admin, and an admin doing so is bound by the same rules — dismissal cancels another user's queued work, so it is not a read-only admin power.
- [ ] T061a [US7] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add a case for the other half of FR-006d: when a user is deleted, their queued downloads leave the queue and never start, and a worker already running is left to finish. T025 covers only the record surviving with a NULL `user_id`.
- [ ] T061b [US7] Handle user deletion in `server/internal/api/ytdl_reconcile.go` — admission must skip rows whose owner is gone, so a deleted account cannot keep consuming slots.
- [ ] T062 [US7] Change `handleYtdlDismiss` in `server/internal/api/ytdl_handlers.go` to stop refusing while running, and to cascade to a group's items (FR-005a).
- [ ] T063 [US7] Add `e2e/stateful/ytdl-queue.spec.ts`: queue more than the limit, assert exactly the limit run and the rest show as queued, and that a finish starts the next.

---

## Phase 8: User Story 5 — Retry the one that failed (P2)

**Goal**: One-action recovery, without re-entering the link.

**Independent test**: Fail a download, retry it, confirm a new attempt starts
against the same link and mode as one row, not two.

**Depends on**: Phase 4, Phase 7.

- [ ] T064 [US5] [TEST] Add `server/internal/api/ytdl_retry_test.go`: retry moves a failed row to `queued`, increments `attempts`, clears `reason` and `finishedAt`, and keeps it one row (FR-029); retry of a non-failed download is 409; retry of another user's is 404.
- [ ] T065 [US5] Create `server/internal/api/ytdl_retry.go` and register `POST /v1/ytdl/{requestId}/retry` in `server/internal/api/router.go`.
- [ ] T066 [US5] [TEST] In `server/internal/api/ytdl_retry_test.go`, add a case asserting retrying a group re-queues only its failed items, not the whole group.
- [ ] T066a [US5] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add a case asserting **nothing retries automatically** (FR-027): a failed download stays `failed` across many reconciler ticks and is never re-admitted without an explicit retry. The queue re-admits by design and `BackoffLimit` is 0, so this regression would otherwise be silent.
- [ ] T067 [P] [US5] Add a retry action to `src/components/YtdlItem.vue` and `src/components/YtdlDetailModal.vue`, offered only in the `failed` state (FR-028).
- [ ] T068 [P] [US5] Add `ytdlRetry` to `src/services/api.ts` and `src/composables/useYtdl.ts`.
- [ ] T069 [US5] Show the attempt count in `src/components/YtdlDetailModal.vue` so a repeat attempt is visible as such (US5 scenario 3).
- [ ] T070 [US5] Extend `e2e/stateful/ytdl.spec.ts` with a fail → retry → runs-again path.

---

## Phase 9: User Story 6 — A playlist or channel becomes its own items (P2)

**Goal**: Expansion into individually trackable downloads, with no ceiling.

**Independent test**: Submit a playlist; it resolves into one download per entry,
each independently trackable and retryable.

**Depends on**: Phase 2, Phase 5, Phase 7.

- [ ] T071 [P] [US6] [TEST] Add `server/internal/ytdl/expand_test.go` — table-driven: entry lines parse; a malformed line is skipped without failing the run; **every derived URL goes back through `Classify`** so an entry pointing off-allowlist is refused (FR-016a); entries are identified by stable id, not link text (FR-016b).
- [ ] T072 [US6] Create `server/internal/ytdl/expand.go`: the listing-mode argv, the entry-line template constant, and the parser. Pure.
- [ ] T073 [US6] [TEST] Add `server/internal/ytdl/job_test.go` cases for the expansion Job: it mounts **no** media library, carries its own deadline, and is labelled so the selector finds it.
- [ ] T074 [US6] Add `BuildExpansionJob` to `server/internal/ytdl/job.go`.
- [ ] T075 [US6] Add the six-state model to `server/internal/ytdl/job.go` — `resolving`, `queued`, `scheduled`, `downloading`, `completed`, `failed` — replacing the four of spec 0012, with failure still checked before success everywhere.
- [ ] T076 [US6] [TEST] Add cases for the legal transitions of `data-model.md`, including that `failed → queued` is the only edge out of a final state and only on an explicit action.
- [ ] T077 [US6] Add expansion to `server/internal/api/ytdl_reconcile.go`: run the listing Job for a `resolving` request, read its entries, skip items already held (FR-020), and insert the rest in one transaction.
- [ ] T078 [US6] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add a case asserting a dismissed record means an item is no longer held and is fetched again (FR-020a).
- [ ] T079 [US6] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, add a case asserting a `resolving` request whose expansion worker vanishes reaches `failed` rather than waiting forever (FR-013d).
- [ ] T079a [US6] [TEST] In `server/internal/ytdl/expand_test.go`, add a case asserting **no ceiling** on expansion (FR-017): a large entry list yields one record per entry with no truncation, no cap constant, and no silent drop. This was an explicit product decision and nothing else guards it.
- [ ] T079b [US6] [TEST] In `server/internal/api/ytdl_reconcile_test.go`, extend T079's coverage to the other way a download can sit forever (FR-013d): a `scheduled` download whose pod never appears must reach `failed` rather than waiting indefinitely.
- [ ] T080 [US6] Add group state derivation and aggregate counts to `server/internal/store/ytdl_repos.go`.
- [ ] T081 [US6] [TEST] Add `server/internal/api/ytdl_detail_test.go` cases for `GET /v1/ytdl/{requestId}/items` — paged items plus the group aggregate; 404 for a group the caller may not see.
- [ ] T082 [US6] Add the group-items handler to `server/internal/api/ytdl_detail.go` and register the route.
- [ ] T083 [US6] Exclude items from the top-level list in `server/internal/api/ytdl_handlers.go` (FR-019b) so a large channel cannot crowd out other downloads.
- [ ] T084 [P] [US6] Render a group row with aggregate counts in `src/components/YtdlItem.vue`.
- [ ] T085 [P] [US6] Create `src/components/YtdlGroupModal.vue` listing a group's items, each openable, retryable and dismissable.
- [ ] T086 [US6] Add `e2e/stateful/ytdl-expand.spec.ts`: a playlist expands to per-entry rows behind one group row; one entry failing leaves the others unaffected; the Tasks list gains exactly one row.

---

## Phase 10: User Story 8 — Nothing lands under Unknown Artist (P3)

**Goal**: Correct artist, album and cover art on every saved item.

**Independent test**: Download an item with no artist or album metadata from a
named playlist; it is shelved under the channel and playlist names with art, in
both modes.

**Depends on**: Phase 9.

- [ ] T087 [P] [US8] [TEST] Add `server/internal/ytdl/sanitize_test.go` — table-driven and adversarial: path separators, parent-directory references, absolute paths, leading dots, control characters, an over-long name, an empty result after stripping. **No input may produce a value that escapes the mounted library** (FR-038a).
- [ ] T088 [US8] Create `server/internal/ytdl/sanitize.go`. This is defence in depth by intent — the extractor's own filename sanitising stays, and neither alone is the argument.
- [ ] T089 [US8] [TEST] Add `server/internal/ytdl/command_test.go` cases asserting the group name reaches the worker as a discrete argument, never concatenated (FR-038), that album falls back playlist/channel → generic (FR-036), and that the folder template and the tag template remain the same expression. Also assert artist still falls back to the uploader (FR-035) — that works today and has no test, and T090 edits the very same expression.
- [ ] T090 [US8] Change `tmplAlbum` and the metadata arguments in `server/internal/ytdl/command.go` to take the sanitised group name, preserving the folder/tag identity that keeps a track from being foldered as one thing and tagged as another.
- [ ] T091 [US8] Pass the group name through `JobConfig` in `server/internal/ytdl/job.go` for expanded items (FR-037).
- [ ] T092 [US8] **Experiment (R9)**: run the music-video path against the pinned `jauderho/yt-dlp:2026.08.19` and inspect the output file for embedded cover art. Record the result in `research.md`.
- [ ] T093 [US8] Depending on T092, either keep embedded art or add a sidecar poster for the video path in `server/internal/ytdl/command.go` (FR-034 accepts either).
- [ ] T094 [US8] **Experiment (R6)**: confirm whether the extractor sanitises a value injected via metadata parsing. Record in `research.md`. This does not change what is built — T088 is required regardless — it records whether there is a second layer.
- [ ] T095 [US8] **Experiment (R2)**: confirm progress fields are usable on the audio path through post-processing, and that a track does not appear stuck at 100% during extraction. Record in `research.md`.

---

## Phase 11: Notifications

**Purpose**: FR-025a–d. No user story owns this; it spans single downloads and groups.

**Depends on**: Phase 9.

- [ ] T096 [TEST] Add `server/internal/api/ytdl_notify_test.go`: a direct download notifies as one download; a group notifies at most once, when every item is final, with saved and failed counts; a group of 340 items produces exactly one notification (SC-005b); notification respects ownership and the user's existing scope.
- [ ] T097 Create `server/internal/api/ytdl_notify.go` honouring the existing `notification_prefs` (added / completed / failed) rather than introducing a second set of switches.
- [ ] T098 Hook group-final detection into `server/internal/api/ytdl_reconcile.go`.
- [ ] T099 [TEST] In `server/internal/api/ytdl_notify_test.go`, add a case asserting a group with one item stuck cannot notify, and that FR-013d's bound is what eventually lets it.

---

## Phase 12: Polish & Cross-Cutting

- [ ] T100 [P] Verify no source link, saved path, library path, playlist name, or raw worker output appears in any log, metric or error payload (FR-032a). Grep the new code and add a test where a handler builds an error string.
- [ ] T100a [P] Confirm `specs/0013-youtube-downloads-managed/contracts/http-api.md` still enumerates every user- or source-influenced value that reaches a worker (FR-038c), and that each is covered by the FR-038 / FR-038a / FR-038b tests. If implementation added a value the list does not name, add it in both places.
- [ ] T101 [P] [TEST] In `server/internal/api/ytdl_handlers_test.go`, add cases asserting an orchestrator or log-read failure is reported distinctly from a download having failed (FR-032b), and that `degraded` keeps its spec 0012 meaning.
- [ ] T102 [P] [TEST] In `server/internal/api/ytdl_handlers_test.go`, add a case asserting the queue reveals nothing cross-user to a non-admin (FR-009c).
- [ ] T103 [P] Confirm the list stays responsive at several thousand records (SC-006a) — seed the store and measure; the page query must not load the whole history.
- [ ] T104 [P] Update `CLAUDE.md`: the six states, the reconciler, the queue, `pods/log`, and `YTDL_MAX_PARALLEL`. The existing text describes spec 0012's four states and "no server-side queue".
- [ ] T105 [P] Update `docs/UPGRADING.md` with the operator-facing change: a new environment variable and a widened Role.
- [ ] T105a [P] Review `e2e/stateful/ytdl-fab.spec.ts` and `e2e/stateful/ytdl.spec.ts` against the changed submit response and the new states, and update any assertion that encoded spec 0012's four-state model.
- [ ] T106 Run the full gate: `npm run build`, `npm run test:unit:coverage`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:e2e`.
- [ ] T107 Set the spec `Status:` to `in-review` and run `make roadmap`.

---

## Dependencies

```
Phase 1 (Setup) ── T001 gates everything
   │
   ├─→ Phase 3 (US1 ownership) ────────────────→ shippable alone
   │
   ├─→ Phase 4 (US2 records) ──┬──────────────→ shippable with US1
   │                            │
   └─→ Phase 2 (Foundational) ──┤
                                │
                                ├─→ Phase 5 (US3 progress)
                                │      │
                                │      └─→ Phase 6 (US4 detail)
                                │
                                └─→ Phase 7 (US7 queue)
                                       │
                                       ├─→ Phase 8 (US5 retry)
                                       │
                                       └─→ Phase 9 (US6 expansion)
                                              │
                                              ├─→ Phase 10 (US8 naming)
                                              └─→ Phase 11 (notifications)
                                                     │
                                                     └─→ Phase 12 (polish)
```

**Story order deviates from raw priority** where dependency demands it: US7
(queue) precedes US5 (retry) because a retried download re-enters the queue, and
precedes US6 (expansion) because the queue is what makes an uncapped expansion
safe. All three are P2, so no priority is inverted.

## Parallel opportunities

- **Phase 1**: T002/T003/T004 together.
- **Phase 2**: the mock work (T011–T013) runs alongside the k8s client work (T005–T008).
- **Phase 3**: T017, T018, T022 together — different files.
- **Phase 5**: T044/T045 (client) alongside T035–T043 (server).
- **Phase 9**: T084/T085 (client) alongside the server work.
- **Phase 12**: T100–T105 all parallel.
- The four pure modules — `progress.go`, `expand.go`, `queue.go`, `sanitize.go` —
  have no dependency on each other and can be written in parallel once their
  tests exist.

## Implementation strategy

**MVP is Phase 1 + Phase 3.** US1 alone closes a live defect where every user can
see every other user's downloads, depends on nothing but the constitution
amendment, and is a complete shippable increment.

**Second increment: + Phase 4.** History that survives the sweep.

**Third: + Phase 2 and Phase 5.** The progress bar — the most-felt gap in 0012.

Each subsequent phase is a complete increment with its own e2e coverage. The
three experiments (T092, T094, T095) are deliberately placed inside the phases
that depend on them rather than up front, because each has a fallback already
written into the spec and none can block.
