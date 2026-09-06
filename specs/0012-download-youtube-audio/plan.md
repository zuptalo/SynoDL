# Implementation Plan: Save YouTube music and music videos to the library

**Branch**: `feat/0012-download-youtube-audio` | **Date**: 2026-09-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0012-download-youtube-audio/spec.md`

## Summary

A user submits a YouTube link and a mode (Music or Music video). The server
validates the link against a host allowlist, classifies its scope (single video,
playlist, or channel), builds an argv-only worker command from a recipe that was
verified end-to-end before this plan was written, and asks the cluster to run it
as a short-lived Job. The Job mounts one of two operator-configured media
libraries — never both, never the server's own volume — writes the file, and
exits.

The server keeps almost nothing. In-flight state is read back by listing Jobs by
label, so the list is correct across restarts with no bookkeeping to drift. The
single exception is a failure record, written to the existing SQLite store so an
unsuccessful download is still visible after the cluster has swept the Job away.
Successful downloads leave the files as their record.

On the client, these appear as rows in the existing Tasks list, discriminated by
source, exposing only the actions a worker can honour.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: Server stays on stdlib `net/http`. **No new Go module
is added** — the Kubernetes Jobs API is reached with `net/http` + `encoding/json`
over TLS built from the in-cluster CA. Client adds nothing.

**Storage**: The existing single SQLite volume, one additive migration (`0020`)
for failure records only. No second datastore. The two media libraries are
worker-mounted volumes holding the operator's media, not SynoDL state.

**Testing**: `go test ./...` with an `httptest` fake Kubernetes API (mirroring
how `internal/syno` is tested against a fake DSM) and fakes behind a narrow
client interface for handlers; `vitest` for new pure client modules; Playwright
e2e against an in-repo mock orchestrator that never downloads anything.

**Target Platform**: k3s (the operator's existing cluster). The feature degrades
to *unavailable, clearly stated* when the server is not running in a cluster, so
Docker-compose and bare-container deployments keep working unchanged.

**Project Type**: Web application — Vue 3 PWA + Go service, single image.

**Performance Goals**: Submitting a download returns immediately; it never waits
on the cluster round-trip beyond creating the Job. Listing tasks adds at most one
label-selector LIST per refresh, independent of how many downloads have ever run.

**Constraints**: No progress reporting by requirement (FR-017). A download must
reach a terminal state within a finite deadline (FR-020). Nothing may re-download
on its own (FR-021). The server container must not mount the media libraries.

**Scale/Scope**: A home instance: a handful of users, a small number of
concurrent workers, playlists and channels in the tens-to-low-hundreds of items.

## Constitution Check

*GATE: evaluated against constitution v2.1.0. Re-checked after Phase 1 — see
"Post-Design Re-check".*

| Rule (v2.1.0) | How this design satisfies it |
|---|---|
| **Ephemeral workers only** | One Job per request, `restartPolicy: Never`, `backoffLimit: 0`, finite `activeDeadlineSeconds`, `ttlSecondsAfterFinished`. It downloads and exits. No worker is a service, a sidecar, or long-lived. |
| **Media volumes are worker-only** | The two library volumes appear only in the Job pod spec. The `synodl` Deployment is not modified to mount them, and a test asserts the server never resolves a path inside them. |
| **Worker images are pinned** | The image reference is an operator config value with a pinned default tag, never `:latest`, and is documented as a supply-chain bump like any dependency. |
| **Single server image, one state volume** | Unchanged. No sidecar is added to the Deployment; the only new server-side persistence is one additive migration on the existing volume. |
| **Worker orchestration is a credential** | A namespaced ServiceAccount + Role (`jobs`: create/get/list/delete; `pods`: get/list). No ClusterRole, no `secrets`, no `pods/exec`. The token is read from the projected volume, never logged, never returned to a client. |
| **Worker inputs are allowlisted, never shell-interpolated** | The URL is host-allowlisted before a Job is built, and is emitted as its own element of the container `args` array. The one shell snippet in the command is a constant we author; no user value is ever concatenated into it. A unit test asserts a hostile URL cannot escape its argv slot. |
| **Job state belongs to the orchestrator** | In-flight state is derived from a labelled LIST. No table shadows a Job's lifecycle. Only terminal *failures* are recorded (FR-024), which the rule explicitly permits under the one-store rule. |
| **One store, one volume** | Failure records go in the existing SQLite database via migration `0020`. No new datastore. |
| **DSM allowlist** | Untouched. This feature calls no DSM API and does not use the stored NAS connection. |
| **Mock-DSM dev parity** | Extended in spirit: a mock orchestrator means `make start` and e2e still need no cluster and no real hardware. |
| **TDD (Principle II)** | The recipe builder and URL classifier are pure and table-tested first; the k8s client is tested against `httptest`; handlers get sibling `_test.go` files; e2e covers the four states including failure. |
| **Ionic-first UI (Principle VI)** | Rows reuse the existing `TaskItem` shell and stock Ionic; no new widget where an Ionic primitive exists. |
| **Credential-Safety Impact** | Present in the spec, and updated during clarification to cover what the failure record stores. |

**Gate result**: PASS, with two entries in Complexity Tracking that are justified
rather than waived.

## Project Structure

### Documentation (this feature)

```text
specs/0012-download-youtube-audio/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0 — decisions already settled, recorded
├── data-model.md        # Phase 1 — entities, migration 0020, Job labels
├── quickstart.md        # Phase 1 — operator setup + local dev
├── contracts/
│   ├── http-api.md      # the /v1 endpoints this adds
│   └── worker-job.md    # the Job contract: argv, mounts, labels, lifecycle
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```text
server/
├── cmd/
│   ├── synodl/                  # unchanged entrypoint; wires the jobs client
│   ├── synomock/                # unchanged (mock DSM)
│   └── synok8s/                 # NEW mock orchestrator (see research.md)
└── internal/
    ├── k8s/                     # NEW: minimal stdlib Jobs client
    │   ├── config.go            #   in-cluster config discovery
    │   ├── jobs.go              #   create / list-by-label / delete
    │   └── jobs_test.go         #   against httptest fake API server
    ├── ytdl/                    # NEW: the verified recipe, pure
    │   ├── url.go               #   host allowlist + scope classification
    │   ├── url_test.go
    │   ├── command.go           #   argv builder for both modes
    │   ├── command_test.go
    │   ├── job.go               #   Job spec assembly (labels, mounts, limits)
    │   └── job_test.go
    ├── api/
    │   ├── ytdl_handlers.go     # NEW + sibling _test.go, house pattern
    │   └── router.go            # extended
    └── store/
        ├── schema.go            # + migration 0020
        └── ytdl_repos.go        # NEW + sibling _test.go

src/
├── types/task.ts                # Task gains a discriminating source field
├── components/
│   ├── TaskItem.vue             # branches on source for actions/marking
│   └── NewTaskModal.vue         # gains the YouTube mode choice
├── composables/useTasks.ts      # merges the two lists into one feed
└── services/task-sort.ts        # filter/sort aware of both row kinds

e2e/stateful/
└── ytdl.spec.ts                 # NEW: four states incl. failure

deploy/k8s/
├── 10-synodl.yaml               # unchanged Deployment (must NOT mount media)
├── 30-rbac.yaml                 # NEW ServiceAccount + Role + RoleBinding
└── 40-media.yaml                # NEW RWX PVCs for the two libraries
```

**Structure Decision**: Everything lands in existing directories following the
existing conventions — one package per concern under `server/internal`, a sibling
`_test.go` per handler file, pure logic in its own package so it carries unit
tests, and client changes confined to the Tasks feature area. The only new
top-level artifacts are the mock orchestrator binary and two deploy manifests.

### Port allocation (extends the documented block)

| Port | What |
|---|---|
| 8295 | mock orchestrator, dev (`make start`) |
| 8296 | mock orchestrator, e2e stateful stack |

Chosen inside SynoDL's own block so it stays clear of sibling projects, matching
the existing `8291` / `8292` / `8294` mock allocations.

## Phase 0 — Research

All open technical questions were resolved by direct experiment before this plan
(see `research.md` for the evidence and the rejected alternatives). No
`NEEDS CLARIFICATION` remains. In brief:

1. **Do not add `client-go`.** Justified in Complexity Tracking.
2. **Progressive-only video download is not viable** — measured, not assumed.
3. **Merging is a stream copy, not a conversion** — measured.
4. **Sidecar naming differs per media type** — `.lrc` must lose its language
   infix, `.srt` must keep a 2-letter one.
5. **Tags, not folders, drive shelving** — hence the paired `--parse-metadata`.
6. **Channel scoping is a URL choice**, not a filter.

## Phase 1 — Design & Contracts

- `data-model.md` — the failure record and migration `0020`; the Job label set
  that makes a LIST authoritative; the discriminated client task type.
- `contracts/http-api.md` — the new `/v1` endpoints and their shapes.
- `contracts/worker-job.md` — the Job contract: exact argv per mode and scope,
  mounts, labels, security context, and the lifecycle→state mapping.
- `quickstart.md` — operator setup (RBAC, PVCs, image pin) and local dev.

**Agent context**: `CLAUDE.md` carries no `<!-- SPECKIT START/END -->` markers,
so there is no generated block to update. CLAUDE.md does need a manual edit in
this PR for two reasons recorded in the constitution's Sync Impact Report: it
still describes the server as a "stateless, credential-free proxy" (stale since
v2.0.0), and it must gain the worker model and the new ports.

## Post-Design Re-check

Re-evaluated after the Phase 1 artifacts: no gate regressed. The design adds one
new server capability (creating workloads), which the constitution now permits
explicitly and constrains in four places — least-privilege RBAC, allowlisted
argv-only inputs, ephemeral workers, worker-only media mounts — each of which has
a named test in `tasks.md`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| A hand-written Kubernetes client instead of `client-go` | The server needs exactly three operations (create Job, list Jobs by label, delete Job) against one namespace. In-cluster config is two env vars and three files; the calls are JSON over HTTPS. | `client-go` would be the largest dependency in the repo by an order of magnitude, against a server with two direct modules where each is a spec-level decision. It brings a scheme/codec/informer machinery none of which this uses, and a version-skew policy tied to cluster releases. The stdlib version is small enough to read in one sitting and is tested the same way `internal/syno` already is. |
| A mock orchestrator binary (`cmd/synok8s`) | The Domain Constraints require that `make start` and e2e never need real hardware; the cluster API is now a hard dependency in exactly the way DSM already was. | Testing against a real cluster breaks the dev-parity rule and makes e2e non-hermetic. Faking only at the Go interface level would leave the wire format, the label selector, and the lifecycle→state mapping untested end-to-end — which is precisely where this feature's bugs will live. It follows the proven `cmd/synomock` pattern rather than inventing one. |
| A second writable volume class (RWX) in deploy | Concurrent workers write the same library; the existing `local-path` RWO PVC cannot be mounted by pods across nodes. | RWO would serialise or fail scheduling as soon as two downloads overlap. This is an operator-provided NFS/CIFS share, not new SynoDL state, and the constitution now permits it explicitly as a worker-only media volume. |
