# Tasks: Save YouTube music and music videos to the library

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-06

## Format: `[ID] [P?] [Story] Description`

- `[P]` = parallelisable (different files, no ordering dependency)
- `[US1]`…`[US4]` = the user story the task serves; `[F]` = foundational

Per Principle II, a failing test precedes the implementation that satisfies it.
Every task names the exact file it touches.

## Phase 1: Setup

- **T001** `[F]` (FR-025) Add instance-wide config for the feature: worker image (pinned tag, no
  `:latest` default), the two library mount paths, library uid/gid, job deadline,
  TTL, and concurrency limit — `server/internal/config/config.go`. Absent config
  must mean "feature unavailable", never a broken default.
- **T002** `[P]` `[F]` Reserve dev ports 8295 / 8296 for the mock orchestrator in
  the port table — `CLAUDE.md`.

## Phase 2: Foundational (blocking prerequisites)

### URL handling — the security boundary

- **T003** `[F]` Failing table test for the host allowlist and scope classifier:
  accepts youtube.com / youtu.be / music.youtube.com; rejects look-alikes
  (`youtube.com.evil.tld`, `evil.tld/youtube.com`, userinfo tricks, non-http
  schemes); classifies single / playlist / channel; normalises a channel URL to
  its videos tab — `server/internal/ytdl/url_test.go`.
- **T004** `[F]` Implement it to green — `server/internal/ytdl/url.go`.

### Command builder — the verified recipe

- **T005** `[F]` Failing table test for the argv builder covering both modes and
  all three scopes, asserting: the output template produces
  `artist/album-or-Singles/title` (FR-007); the title segment is the **verbatim**
  source title with no stripping or normalisation (FR-008); the `-o` template and
  the album `--parse-metadata` template are byte-identical and both metadata
  flags are present (FR-009); thumbnail embedding and jpg conversion are present
  (FR-010); original-language captions are requested and converted to the right
  companion format per mode (FR-011); `playlist_title` appears in NO metadata
  fallback chain; the duration filter has a lower bound and no upper bound
  (FR-013); the archive path sits inside the target library (FR-015) —
  `server/internal/ytdl/command_test.go`.
- **T006** `[F]` **Injection test** (constitution v2.1.0): for hostile URLs
  (`; rm -rf /`, backticks, `$(…)`, newlines, quotes) assert the URL appears as
  exactly ONE element of `args`, byte-equal to the normalised input, and that no
  other element contains it as a substring — `server/internal/ytdl/command_test.go`.
- **T007** `[F]` Implement the builder to green — `server/internal/ytdl/command.go`.

### Kubernetes client

- **T008** `[P]` `[F]` Failing test for in-cluster config discovery: env vars +
  projected token/CA/namespace; a clear "not in a cluster" sentinel error when
  absent — `server/internal/k8s/config_test.go`.
- **T009** `[F]` Implement in-cluster config — `server/internal/k8s/config.go`.
- **T010** `[F]` Failing test against an `httptest` fake API server for create /
  list-by-label-selector / delete, including non-2xx and malformed-body handling,
  and asserting the bearer token never appears in a returned error string —
  `server/internal/k8s/jobs_test.go`.
- **T011** `[F]` Implement the Jobs client — `server/internal/k8s/jobs.go`.

### Job assembly + state mapping

- **T012** `[F]` (FR-004, FR-005, FR-006, FR-020, FR-021, FR-022) Failing test for Job spec assembly: labels and annotations per
  the data model; `restartPolicy: Never`; `backoffLimit: 0`; finite
  `activeDeadlineSeconds`; TTL; runAsUser/runAsGroup; `XDG_CACHE_HOME=/tmp`;
  `automountServiceAccountToken: false`; and **exactly one** library volume
  mounted at `/out` — with an explicit case asserting a `music` job does not
  reference the music-video path at all — `server/internal/ytdl/job_test.go`.
- **T013** `[F]` Failing test for the lifecycle→state mapping table from the data
  model, including `DeadlineExceeded` → failed and **vanished-without-condition →
  NOT completed** — `server/internal/ytdl/job_test.go`.
- **T014** `[F]` Implement Job assembly and state mapping — `server/internal/ytdl/job.go`.

### Store

- **T015** `[F]` Add migration `0020` for `ytdl_failures` per the data model —
  `server/internal/store/schema.go`. Append only; never edit a shipped migration.
- **T016** `[F]` Failing test for the failure repo: idempotent upsert by
  `request_id`, list, delete, bounded pruning, and `ON DELETE SET NULL` on user
  removal — `server/internal/store/ytdl_repos_test.go`.
- **T017** `[F]` Implement the repo — `server/internal/store/ytdl_repos.go`.
- **T018** `[F]` Extend the migrations golden test for `0020` —
  `server/internal/store/migrations_golden_test.go`.

### Mock orchestrator

- **T019** `[P]` `[F]` Implement the mock orchestrator: create / list-by-selector
  / delete plus the `/__mock/*` lifecycle controls from the worker-job contract,
  including `vanish` and `reset`. It must never download anything —
  `server/cmd/synok8s/main.go` + `server/internal/k8smock/`.
- **T020** `[F]` Wire it into `make start` on :8295 — `Makefile`, `server/Makefile`.

## Phase 3: User Story 1 — Save a song to the music library (P1) 🎯 MVP

- **T021** `[US1]` (FR-001, FR-003, FR-023) Failing handler test for `POST /v1/ytdl` in music mode against
  a fake jobs client: 202 + requestId; 400 on a blocked host; 409 on duplicate
  in-flight; 503 with no orchestrator — `server/internal/api/ytdl_handlers_test.go`.
- **T022** `[US1]` Implement `POST /v1/ytdl` — `server/internal/api/ytdl_handlers.go`.
- **T023** `[US1]` Failing handler test for `GET /v1/ytdl`: one LIST call
  regardless of row count; stored failures merged; `degraded` true when the
  orchestrator is unreachable; **no progress fields present in the JSON at all** —
  `server/internal/api/ytdl_handlers_test.go`.
- **T024** `[US1]` Implement `GET /v1/ytdl`, recording observed failures
  idempotently — `server/internal/api/ytdl_handlers.go`.
- **T025** `[US1]` Register both routes behind the session gate —
  `server/internal/api/router.go`.
- **T026** `[P]` `[US1]` Client: add the `source` discriminator to the task type
  and the ytdl API calls — `src/types/task.ts`, `src/services/api.ts`.
- **T027** `[US1]` (FR-026) Client: merge the ytdl feed into the task list —
  `src/composables/useTasks.ts`.
- **T028** `[US1]` (FR-001) Client: mode choice in the new-task flow, submitting a link as
  Music — `src/components/NewTaskModal.vue`.
- **T029** `[US1]` (FR-026) Client: render a ytdl row with its source marker and the four
  states, using stock Ionic — `src/components/TaskItem.vue`.
- **T030** `[US1]` e2e: submit in music mode, drive the mock to succeed, assert
  the row shows scheduled → started → completed — `e2e/stateful/ytdl.spec.ts`.

**Checkpoint**: music downloads work end to end.

## Phase 4: User Story 2 — Save a music video (P1)

- **T031** `[US2]` (FR-012) Extend the argv-builder test for music-video mode: the format
  selector prefers avc1+mp4a with the documented fallback chain, mp4 merge
  output, srt subtitles — `server/internal/ytdl/command_test.go`.
- **T032** `[US2]` Extend the builder — `server/internal/ytdl/command.go`.
- **T033** `[US2]` Failing test that a music-video job mounts only the
  music-video library — `server/internal/ytdl/job_test.go`.
- **T034** `[P]` `[US2]` Client: offer Music video as the second mode —
  `src/components/NewTaskModal.vue`.
- **T035** `[US2]` e2e: submit in music-video mode and assert the row is marked as
  a video download — `e2e/stateful/ytdl.spec.ts`.

**Checkpoint**: both modes work; each writes only its own library.

## Phase 5: User Story 3 — Playlists and channels (P2)

- **T036** `[US3]` Extend the scope tests: playlist and channel argv (FR-002),
  channel URL normalised to the videos tab (FR-014), archive path inside the
  target library (FR-015), and the continue-on-unavailable-item flag present for
  both bulk scopes so one dead entry cannot fail the run (FR-016) —
  `server/internal/ytdl/url_test.go`, `command_test.go`.
- **T037** `[US3]` Implement the remaining scope handling —
  `server/internal/ytdl/url.go`, `command.go`.
- **T038** `[US3]` Client: show the derived scope on the row —
  `src/components/TaskItem.vue`.
- **T039** `[US3]` e2e: submit a channel URL and assert the request is accepted
  with scope `channel` — `e2e/stateful/ytdl.spec.ts`.

**Checkpoint**: all three scopes accepted and correctly shaped.

## Phase 6: User Story 4 — Lifecycle visibility (P2)

- **T040** `[US4]` Failing test: a job that vanishes without a terminal condition
  is reported failed when a failure record exists and is otherwise not shown —
  never completed — `server/internal/api/ytdl_handlers_test.go`.
- **T041** `[US4]` Implement `DELETE /v1/ytdl/{requestId}`: 204 / 404 / 409 while
  in flight, and never touching the library —
  `server/internal/api/ytdl_handlers.go`, `router.go`.
- **T042** `[US4]` Client: filter and sort handle both row kinds —
  `src/services/task-sort.ts`, `src/composables/useTaskFilter.ts`.
- **T043** `[US4]` Client: bulk NAS actions skip ytdl rows and report the count
  skipped (FR-028) — `src/views/tabs/TasksPage.vue`.
- **T044** `[US4]` Client: a ytdl row never offers pause or resume (FR-027) —
  `src/components/TaskItem.vue`.
- **T045** `[US4]` e2e: drive the mock to fail and to deadline-exceed, asserting
  both read as failed and neither as completed; and that a failure survives a
  server restart — `e2e/stateful/ytdl.spec.ts`.

**Checkpoint**: the full four-state model, with failure distinguishable.

## Phase 7: Deploy & docs

- **T046** `[P]` ServiceAccount + namespaced Role + RoleBinding; assert by review
  that it is not a ClusterRole and grants no `secrets` or `pods/exec` —
  `deploy/k8s/30-rbac.yaml`.
- **T047** `[P]` RWX PVCs for the two libraries, with a header comment explaining
  why RWO/local-path is wrong here — `deploy/k8s/40-media.yaml`.
- **T048** Wire the feature's config into the Deployment WITHOUT mounting either
  media volume on the server container — `deploy/k8s/10-synodl.yaml`.
- **T049** `[P]` `CLAUDE.md`: document the worker model, the new ports, and the
  two new packages — and fix the stale v2.0.0 framing at lines 13 and 145 that
  still calls the server a "stateless, credential-free proxy".
- **T050** `[P]` Operator setup + rollback notes — `docs/` and `quickstart.md`.

## Phase 8: Gates

- **T051** `npm run build` (typecheck + build) green.
- **T052** `npm run test:unit:coverage` green, floors not regressed.
- **T053** `cd server && go build ./... && go vet ./... && go test ./...` green.
- **T054** `npm run test:e2e` green.
- **T055** `make roadmap` current; spec Status moved to `in-review`.

## Dependencies & Execution Order

- Phase 1 → Phase 2 → user-story phases → Phase 7 → Phase 8.
- Phase 2 is genuinely blocking: every story depends on the URL boundary, the
  command builder, the k8s client, and the store.
- US1 is the MVP and must be complete before US2–US4 start; US2, US3, US4 are
  independent of each other once US1 lands.
- `[P]` tasks within a phase touch disjoint files and may run together.

## Constitution mapping

| Rule | Enforcing task |
|---|---|
| Worker inputs allowlisted + argv only | T003, T004, T006 |
| Media volumes worker-only, one per job | T012, T033, T048 |
| Least-privilege namespaced RBAC | T046 |
| Ephemeral workers | T012 |
| Job state owned by the orchestrator | T013, T023, T040 |
| One store | T015–T018 |
| Dev/e2e parity, no real hardware | T019, T020, T030 |
| TDD | every implementation task follows its test task |

## Analysis pass (2026-09-06)

Ran before implementation, per Principle I.

**Finding 1 — traceability gaps (fixed).** Fourteen requirements had no
downstream reference. Ten were genuinely covered but unannotated; annotations
added so `analyze` can verify coverage mechanically rather than by reading.

**Finding 2 — four requirements had no named assertion (fixed).** FR-008
(verbatim title), FR-010 (cover art), FR-011 (captions), and FR-016 (continue
past an unavailable item) were implied by "implement the recipe" but nothing
would have failed if they regressed. They are now explicit assertions in T005 and
T036 — and they are the four most likely to rot silently, since each still
produces a plausible-looking file when wrong.

**Finding 3 — success criteria are covered but not 1:1 mapped.** SC-001…SC-009
are end-to-end outcomes verified by the e2e tasks (T030, T035, T039, T045) and
the operator verification in `quickstart.md`. SC-003 (no re-encode) is the one
that cannot be asserted in a hermetic e2e run, because the mock never downloads;
it is verified by the argv assertion in T031 plus manual verification against the
real cluster, and this is called out rather than papered over.

**Result**: clean. No unresolved findings block implementation.
