# Implementation Plan: YouTube downloads you can watch, keep, and retry

**Branch**: `feat/0013-youtube-downloads-managed` | **Date**: 2026-09-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/0013-youtube-downloads-managed/spec.md`

## Summary

Turn spec 0012's fire-and-forget worker jobs into downloads a user manages: a
durable record of every request, live progress read from the worker's own
output, per-item expansion of playlists and channels, an instance-wide queue of
four with fair-share between users, user-initiated retry, a detail view, and
per-user visibility with an admin override.

The technical shape, from `research.md`: **one reconciler ticker** in the server
does everything periodic — lists this feature's Jobs, reads running workers'
output through a newly granted `pods/log`, captures terminal facts durably while
they are still readable, and admits from the queue. The worker's progress is a
format SynoDL defines (`--newline` + `--progress-template` behind a sentinel
prefix), not scraped text. A playlist or channel becomes its own short-lived
listing Job whose entries the server reads back and turns into one record each.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript / Vue 3 + Ionic (client)

**Primary Dependencies**: stdlib `net/http`; `modernc.org/sqlite`. **No new Go
module.** Reading pod logs and listing pods are two more calls on the existing
hand-written `internal/k8s` client — the reason that package exists rather than
`client-go` is unchanged by this feature.

**Storage**: the single SQLite database on the one state volume. One appended,
re-runnable migration adding one table. No second datastore.

**Testing**: `go test ./...` against a fake `JobRunner` and an `httptest` fake
API server; vitest for pure client modules; Playwright e2e on the existing
stateful stack against `internal/k8smock`. No cluster, no NAS, no network.

**Target Platform**: single container serving the PWA and `/v1`; Kubernetes for
worker Jobs, with every ytdl endpoint answering 503 outside a cluster.

**Project Type**: web application — Vue PWA + Go service, one repo, one image.

**Performance Goals**: list stays responsive at thousands of records (SC-006a);
worker-output reads bounded in size and frequency and independent of viewer
count (FR-013e/f); a second user's single link waits at most one running
download (SC-005a).

**Constraints**: instance-wide limit of 4, operator-configurable; unbounded
history; single replica, so one admitter and no distributed lock; the server
never mounts a media library.

**Scale/Scope**: one channel may expand to thousands of records. 7 server
packages touched, 1 migration, 7 endpoints, 3 client surfaces.

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1.*

| Principle | Verdict | Basis |
|---|---|---|
| **I. Spec-Driven Development** | PASS | specify → clarify → checklist → plan, in order. `analyze` before `implement` still pending. |
| **II. Test-Driven Development** | PASS with obligation | Every new pure module (progress parsing, expansion parsing, fair-share ordering, name sanitising, state derivation) is table-testable with no cluster. `tasks.md` MUST order those tests before their implementations. Client coverage floors may rise, never regress. |
| **III. Custodial State & Credential Safety** | **CONDITIONAL — amendment required** | See below. |
| **IV. Offline-first client data** | PASS | Downloads are not persisted client-side; the server remains the source of truth and the app polls, exactly as for NAS tasks. |
| **V. Quality Gates** | PASS | Standard gates. Release-note subjects must be plain-language and carry no spec/FR references. |
| **Domain: ephemeral workers only** | PASS | The expansion worker starts, lists, exits. It is not a service and holds no state. |
| **Domain: media volumes are worker-only** | PASS | Unchanged; the expansion worker mounts no library at all. |
| **Domain: worker images pinned** | PASS | Same pinned image, no new one. |
| **Domain: single server image, one state volume** | PASS | One appended migration, one table, no second datastore. |

### Principle III: what must be amended, and why it is a widening not a relaxation

Three rules in Principle III as written do not admit this feature:

1. *"In-flight worker state is derived … never mirrored into the SQLite store."*
   A **queued** download has no worker, so recording it mirrors nothing — but the
   text does not say so. The amendment must make explicit that pre-admission work
   is durable, and that live state for an admitted download is still derived.
2. *"Where a spec needs a durable record of finished work…"* is written as a
   carve-out for failures. This spec records successes too.
3. Least-privilege worker orchestration enumerates what the credential must not
   reach. Reading a worker's own output is new and must be named — as permitted,
   and as still excluding secrets, exec and attach.

**Bump**: MINOR, 2.1.0 → 2.2.0. No principle is removed or reversed; the custody
rules gain cases and keep every prohibition.

**Sequencing** (FR-032's neighbour in Credential-Safety Impact): the amendment
lands **before or with** the first task that depends on it. Principle I makes
code without an approved constitutional basis a defect, so it cannot follow the
work. It is therefore task 1.

### Post-Phase-1 re-evaluation

Re-checked after `data-model.md` and `contracts/` were written. No new violation
appeared, and one prior concern resolved: holding progress **in memory** rather
than in the store (R8) means the "never mirrored" rule needs no further
widening than the three points above — progress is a cache of a reading, lost on
restart without consequence.

The credential-safety checklist (`checklists/security.md`) is complete: 38 of 38
evaluated, 22 of which changed the spec.

## Project Structure

### Documentation (this feature)

```text
specs/0013-youtube-downloads-managed/
├── spec.md
├── plan.md                    # this file
├── research.md                # Phase 0
├── data-model.md              # Phase 1
├── contracts/http-api.md      # Phase 1
├── quickstart.md              # Phase 1
├── checklists/
│   ├── requirements.md        # spec quality
│   └── security.md            # constitution Principle III gate
└── tasks.md                   # /speckit-tasks — NOT created here
```

### Source code

```text
.specify/memory/constitution.md      # amended FIRST (2.1.0 → 2.2.0)

server/internal/k8s/
├── types.go                         # + PodList, Pod
├── jobs.go                          # + ListPods(selector), + PodLog(name, opts)
└── (package doc updated: no longer "Jobs and nothing else")

server/internal/ytdl/
├── command.go                       # + --newline/--progress-template; album from group name
├── progress.go        NEW           # parse our own sentinel lines → readings   [pure, table-tested]
├── expand.go          NEW           # expansion Job + parse entry lines         [pure, table-tested]
├── sanitize.go        NEW           # group-name sanitising (FR-038a/b)         [pure, table-tested]
├── queue.go           NEW           # fair-share admission ordering             [pure, table-tested]
└── job.go                           # + group/item labels, expansion Job shape

server/internal/store/
├── schema.go                        # + migration: ytdl_downloads (append-only, re-runnable)
└── ytdl_repos.go                    # + Download CRUD, paging, already-held, group aggregate

server/internal/api/
├── ytdl_handlers.go                 # ownership filter, paging, submit → queue
├── ytdl_detail.go     NEW           # GET detail, GET group items
├── ytdl_retry.go      NEW           # POST retry
├── ytdl_reconcile.go  NEW           # the one ticker: list → read output → capture → admit
├── ytdl_notify.go     NEW           # group-level notification (FR-025a–d)
├── ytdl_thumb.go                    # + require a session (FR-009d)
└── router.go                        # new routes

server/internal/config/config.go     # + YTDL_MAX_PARALLEL (default 4)
server/internal/k8smock/k8smock.go   # + pods, pod logs, progress/expansion controls
server/cmd/synodl/main.go            # start the reconciler ticker

src/services/api.ts                  # new endpoints + types
src/composables/useYtdl.ts           # paging, progress, group items, retry
src/utils/format.ts                  # + fixed yyyy-mm-dd / HH:MM:SS formatter   [pure, unit-tested]
src/components/YtdlItem.vue          # progress bar, group row with counts
src/components/YtdlDetailModal.vue   NEW   # matches TaskDetailModal convention
src/components/YtdlGroupModal.vue    NEW   # a group's items

deploy/k8s/30-rbac.yaml              # + pods/log: get  (still a namespaced Role)

e2e/stateful/
├── ytdl.spec.ts                     # extended
├── ytdl-progress.spec.ts  NEW
├── ytdl-expand.spec.ts    NEW
├── ytdl-queue.spec.ts     NEW
└── ytdl-ownership.spec.ts NEW
```

**Structure Decision**: the existing monorepo layout, unchanged. New server logic
lands as **pure, table-tested modules in `internal/ytdl`** (progress, expansion,
sanitising, fair-share) with thin handlers over them — matching how `command.go`,
`url.go` and `job.go` already carry this feature's decisions. The client gets two
modals rather than routes, matching `TaskDetailModal.vue` (R11).

## Implementation sequencing

Ordered so each stage is independently shippable and testable, and so the
riskiest unknowns are proven early rather than discovered late.

| # | Stage | Delivers | Depends on |
|---|---|---|---|
| 1 | **Constitution amendment** 2.1.0 → 2.2.0 | The basis for everything below | — |
| 2 | **Ownership fix** (US1) | The live defect closed; smallest shippable slice | 1 |
| 3 | **Durable records + migration** (US2) | Every download recorded; history survives the sweep | 1 |
| 4 | **`pods/log` + output reading** | Progress and lyrics/language (US3) | 1, 3 |
| 5 | **Detail view + fixed date format** (US4) | Every gathered fact visible | 3, 4 |
| 6 | **Queue + fair-share** (US7) | The limit, durable across restart | 3 |
| 7 | **Retry** (US5) | Recovery in one action | 3, 6 |
| 8 | **Expansion** (US6) | Playlists and channels become items | 4, 6 |
| 9 | **Naming + artwork** (US8) | Nothing under Unknown Artist | 8 |
| 10 | **Notifications** | One per group | 8 |

Stage 4 carries the two experiments from `research.md` (R2 audio-path fields, R9
mp4 cover art). Both have a fallback written into the spec, so neither can block
— but both are proven at stage 4/9 rather than assumed.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Principle III amended (durable queue, records of successes, worker-output read) | The spec's core asks — a visible queue, history that survives the sweep, and a real progress bar — are each impossible under the text as written | Keeping the rules and dropping the features was offered to the user and declined. A worker callback avoids the `pods/log` grant but needs our own worker image, a per-job credential in the pod, and a network path back — strictly more credential surface, not less |
| A second worker kind (expansion) | The server has no extractor and must not download; enumerating a channel is extraction | Enumerating in the server means shipping an extractor in the server image. The site's data API means a new API-key credential and a quota |
| An in-memory progress cache | FR-013f forbids reads that scale with viewers; the store must not hold in-flight state | Reading in the request handler violates FR-013f. Storing progress violates Principle III's derived-not-mirrored rule |
| `pods/log` added to the Role | Progress and companion-file facts exist only in worker output | Every alternative source (callback, shared volume, Job status) needs more credential surface or does not carry the data |
