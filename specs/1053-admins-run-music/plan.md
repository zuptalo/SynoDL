# Implementation Plan: Admins run the music library repair from Settings

**Branch**: `feat/1053-admins-run-music` | **Date**: 2026-09-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/1053-admins-run-music/spec.md`

## Summary

Put the spec 1052 repair tool behind an admin-only Settings section. The server
launches the tool as the same kind of one-shot worker Job it already launches for
downloads, learns about the run only from that Job and a small, bounded, parsed set
of machine-readable lines the tool prints, and shows it in a modal.

Five decisions carry the design (each is argued in [research.md](research.md)):

1. **The worker's code is the server's own image, by digest.** An init container in
   the worker pod runs the SynoDL image the server itself is running and copies
   `/opt/music_repair` into an `emptyDir`; the pinned worker image then runs it. Code
   and server cannot disagree on version (FR-018), nothing is duplicated or embedded,
   and no ConfigMap or new permission is needed. The server learns its own image from
   its own pod (`pods: get` is already granted).
2. **The tool reports through a fixed event protocol** (`@@synodl {json}` lines:
   `progress`, `result`). The server tails the pod log (bounded), keeps only lines
   with that prefix, decodes them into a fixed Go struct and drops everything else.
   Nothing raw is returned or logged (FR-016, FR-022).
3. **One reconciler loop.** Capturing a finished run's outcome (before the Job is
   swept) is a function on the existing download reconciler's tick, not a new loop,
   so nothing races over the same pods list.
4. **"One at a time" is enforced by the database** (a partial unique index on
   `state='running'`), then by a Jobs check that also sees a run started from the
   command line, then by the tool's own lock on the share.
5. **RBAC is unchanged.** Jobs create/list/delete, pods get/list, `pods/log` get —
   exactly what the download workers already use.

## Technical Context

**Language/Version**: Go 1.27 (server, stdlib `net/http`), TypeScript / Vue 3 + Ionic 9 (client), Python 3.14 (the existing tool, extended).

**Primary Dependencies**: none new. `internal/k8s` gains `InitContainers`, read-only mounts and a `GetPod` call (types and one GET); the tool gains an event emitter (stdlib).

**Storage**: the existing single SQLite store: one new table `music_repairs` (migration 0040). Live state is derived from Jobs; only the request and its finished outcome are stored (constitution Principle III). Nothing on the library volume changes format.

**Testing**: Go unit tests against a fake `JobRunner` (parser, state machine, handlers, reconciler capture, store); Python unit tests for the event emitter; Vitest for the pure client state module; Playwright e2e on the stateful stack against the mock Kubernetes API (`synok8s`), which already supports Jobs, pod logs, `/__mock/jobs/{name}/emit` and succeed/fail/vanish.

**Target Platform**: k3s; phone-first PWA.

**Project Type**: web application (client + Go service) plus the existing operator tool.

**Performance Goals**: a run's progress shown within one poll (3 s) of the worker printing it; opening the section is one request.

**Constraints**: server never mounts the library; worker output bounded (256 KiB read, ≤ 64 event lines parsed, every string length-capped); no new RBAC; worker image pinned; outside a cluster / without a music claim / without a resolvable own image the endpoints answer `available:false` with a reason and nothing else changes.

**Scale/Scope**: a handful of runs a month; history capped at 50 rows; one active run.

## Constitution Check

| Principle / constraint | Status |
|---|---|
| I Spec-driven | Spec 1053, full pipeline; checklist required (worker orchestration, credential boundary). |
| II TDD | Tasks order failing tests first. New/changed HTTP handlers get Go unit tests; the new user-facing behaviour gets an e2e spec under `e2e/stateful/`; the new pure client module joins the vitest coverage allowlist above its floor. |
| III Custodial state (NON-NEGOTIABLE) | One store, one volume: a bounded history table holding kind, plan id, who, times, state and the fixed summary. No second datastore. Live state derived from Jobs, not mirrored. **Worker output**: read via `pods/log` only, bounded, parsed into a fixed shape, never logged, never returned raw — the exact rules of v2.2.0 (spec 0013 precedent). Credentials: none involved; the worker has no service-account token. |
| Worker orchestration | Namespaced Role, verbs unchanged: `jobs` create/list/get/delete, `pods` get/list, `pods/log` get. No exec/attach, no secrets, no ClusterRole. |
| Ephemeral workers only | The repair Job starts, does one unit of work, exits, holds no SynoDL state. |
| Media volumes are worker-only | The server mounts nothing. Only the worker pod mounts the music claim; the init container mounts only an `emptyDir`. |
| Worker images pinned | The main container is the pinned `YTDL_IMAGE`. The init container is SynoDL's OWN image **by digest** (read from the running pod), which is a stricter pin than a tag; it runs only `cp` on files in that image. This reading of the constraint is stated here so review can challenge it. |
| Same-version code | Guaranteed by construction (init container = the running server's image). |
| VI Ionic-first | Stock `ion-modal`, `ion-list`, `ion-item`, `ion-progress-bar`, `ion-checkbox`, `ion-button`, `ion-accordion`; existing `--app-*` tokens only. |
| V Gates / VII delivery | `go build/vet/test`, `npm run build`, vitest floors, e2e; Conventional Commit `feat(settings): …` whose subject is release-note copy; issues per phase and `Closes #N`. |

Gate result: **PASS**, no Complexity Tracking entries. One point flagged for
reviewers: the init container (above).

### Credential-Safety Impact

See the spec's section. Design-level additions: the events the server accepts are a
closed schema (unknown fields and lines are discarded, numbers clamped, strings
length-capped and stripped of control characters); the only free text that reaches a
client is ≤ 20 library-relative example paths, admin-only; nothing from the worker is
logged; the run history holds only that same bounded summary.

## Project Structure

### Documentation (this feature)

```text
specs/1053-admins-run-music/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/  api.md  worker-events.md
├── checklists/  requirements.md  security.md
└── tasks.md
```

### Source Code

```text
scripts/music_repair/                 # existing tool — extended
├── events.py                         # NEW: `@@synodl` event emitter (progress, result), bounded
├── __main__.py  planner.py  applier.py   # emit events at phase boundaries and at the end
└── test_events.py                    # NEW
Dockerfile                            # COPY scripts/music_repair → /opt/music_repair (runtime stage)
server/internal/
├── k8s/
│   ├── types.go                      # + InitContainers, VolumeMount.ReadOnly, Pod spec/status image fields
│   └── pods.go                       # + GetPod
├── musicrepair/                      # NEW package: pure logic, no HTTP
│   ├── job.go                        # BuildJob (init container + worker), labels, names
│   ├── events.go                     # ParseLog → Summary/Progress (closed schema, bounds)
│   ├── state.go                      # derive run state from Job + record; canApply/canUndo rules
│   └── *_test.go
├── store/
│   ├── schema.go                     # migration 0040 music_repairs (+ partial unique index)
│   └── music_repairs.go              # Insert (atomic one-running), Finish, List, Prune, Get
└── api/
    ├── music_repair_handlers.go      # GET/POST /v1/library/repair[/check|/apply|/undo]
    ├── music_repair_reconcile.go     # capture outcomes on the existing reconciler tick
    └── *_test.go
server/cmd/synok8s + internal/k8smock # small: serve GET pod (own pod), keep emit/succeed controls
src/
├── services/api.ts                   # + repair calls and types
├── services/repair-state.ts          # NEW pure module: what to show/offer from a snapshot (+ tests)
├── components/MusicRepairModal.vue   # NEW
└── views/tabs/SettingsPage.vue       # + admin entry
e2e/stateful/music-repair.spec.ts     # NEW
docs/MUSIC-LIBRARY-REPAIR.md          # + "From Settings"; CLAUDE.md pointer
```

**Structure Decision**: the repair logic that is not HTTP lives in a new
`internal/musicrepair` package (like `internal/ytdl`), so it is table-testable
without a server; handlers stay thin, as elsewhere in `internal/api`.

## Phases (build order)

1. **Tool events** (Python, tests first): the protocol, emitted from `plan`/`apply`/`restore`.
2. **k8s plumbing + `musicrepair` package** (pure Go, tests first): types, `GetPod`, `BuildJob`, event parser, state rules.
3. **Store**: migration, atomic one-running insert, finish, prune.
4. **API + reconciler hook**: handlers (admin-only, availability, one-at-a-time, apply guards), outcome capture.
5. **Image + mock + docs**: Dockerfile copy, mock own-pod, runbook.
6. **Client**: api types, pure state module (tests), modal, Settings entry.
7. **e2e + gates + real run**: Playwright on the stateful stack; then a real check on the cluster after deploy.

## Complexity Tracking

None.
