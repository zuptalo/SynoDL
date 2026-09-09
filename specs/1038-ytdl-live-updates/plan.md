# Implementation Plan: YouTube downloads update as they happen

**Branch**: `feat/1038-ytdl-live-updates` | **Date**: 2026-09-09 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/1038-ytdl-live-updates/spec.md`

## Summary

Replace the five-second poll behind YouTube downloads with a server-push stream.
A new `GET /v1/ytdl/stream` carries deltas — only the downloads that changed —
announced by the reconciler at the end of the cycle that learned about them. The
client merges what it holds and keeps the poll as a fallback.

The shape, from `research.md`: **a fingerprint diff over the unfinished set**,
run at the end of `reconcileYtdlOnce` and skipped entirely when nobody is
watching. Two of the four things that make up a download's shown state — whether
its job is running, and its progress percentage — are known only inside that
cycle, so a stream that re-read the store on its own clock would still be waiting
on the reconciler. Diffing the *projected view* rather than the stored row is
what makes derived change and written change look the same.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript / Vue 3 + Ionic (client)

**Primary Dependencies**: stdlib `net/http`. **No new Go module, no new client
dependency.** SSE is written by hand on both sides already (spec 0006).

**Storage**: none added. This feature stores nothing; it changes how what is
already computed reaches a client. One read-only store query is added.

**Testing**: `go test ./...` with a fake `JobRunner` and an in-process hub; the
existing stateful Playwright stack for the end-to-end path. No cluster, no NAS,
no network.

**Target Platform**: single container serving the PWA and `/v1`. Stateful mode
only — YouTube downloads do not exist in the stateless dev path.

**Performance Goals**: SC-002a — ten viewers cost the same database reads as one.
SC-003 — a change in a five-hundred-track channel sends a payload the size of one
row. Idle costs one heartbeat per connection per 15s.

**Constraints**: the reconcile interval (3s) is the floor on freshness and the
spec says so; announcing must never block the reconciler; a slow reader is
dropped, not queued indefinitely.

**Scale/Scope**: 1 new endpoint, 1 new server file plus a watch step, 1 store
query, 2 client surfaces (the Tasks list and the group sheet).

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1.*

| Principle | Verdict | Basis |
|---|---|---|
| **I. Spec-Driven Development** | PASS | specify → clarify → plan, in order. `analyze` before `implement`. |
| **II. Test-Driven Development** | PASS with obligation | The diff and the hub are pure and table-testable with no cluster. `tasks.md` orders each test before its implementation. |
| **III. Custodial State & Credential Safety** | PASS | **No amendment needed.** No new stored data, no new secret, no new worker input, no widened permission, no new DSM API. The session stays in a header (`fetch`, not `EventSource`). Ownership uses the existing `ytdlVisibleTo` and is re-checked on every heartbeat rather than captured once — a stream outlives the request that opened it. Nothing new reaches a log. |
| **IV. Offline-first client data** | PASS | Downloads are still not persisted client-side; the server remains the source of truth. |
| **V. Quality Gates** | PASS | Standard gates. Release-note subject is plain-language with no spec or FR reference. |
| **Domain: ephemeral workers only** | PASS | Untouched. No worker learns anything new. |
| **Domain: job state belongs to the orchestrator** | PASS | Nothing new is mirrored. The diff reads the same derived projection the list endpoint returns and stores none of it; the fingerprint map is in memory and is rebuilt from a cycle, exactly as the progress cache is. |
| **Domain: single server image, one state volume** | PASS | No migration. |

**No credential checklist required.** The constitution's gate triggers on stored
data, secrets, the NAS connection, user auth, the DSM allowlist, or worker and
cluster credentials. This touches none of them.

## Phase 0 — Research

Complete: [research.md](./research.md). Six questions settled, the load-bearing
one being §2 — where a change actually becomes known — which is what puts the
watch step inside the reconcile cycle rather than in the connection.

## Phase 1 — Design

### Server

**`internal/api/ytdl_hub.go`** — the broadcaster.

- `ytdlHub` holds subscribers behind a mutex. `Subscribe(userID, isAdmin)`
  returns a buffered channel and a cancel; `Publish(changes)` fans out.
- Each send is **non-blocking** (FR-012a): a subscriber whose buffer is full is
  closed and dropped (FR-012b), and recovers by reconnecting under FR-004b.
- `hasSubscribers()` so the diff can be skipped when nobody is watching.
- A change carries the owner id, whether it was created or removed, and the view
  **pre-marshalled twice** — as an admin sees it and as its owner does — so
  rendering is per change, not per viewer.

**`internal/api/ytdl_watch.go`** — the diff.

- `watchYtdl()` runs last in `reconcileYtdlOnce`, after every other step has had
  its effect, and returns immediately unless someone is watching.
- Projects every unfinished download through the same `ytdlViewOf` the list
  endpoint uses, marshals it, and compares against the previous cycle's
  fingerprint.
- Ids that left the unfinished set are resolved one by one: present means it went
  final; `ErrNotFound` means it was dismissed and is published as a removal.
- The first cycle seeds the map without publishing, so a restart does not
  announce the whole world as new.

**`internal/api/ytdl_stream.go`** — the endpoint.

- `GET /v1/ytdl/stream`, stateful only, behind `requireUser`.
- Its own `streamLimiter`, sized from the same config as the NAS one but separate
  from it, so YouTube viewers cannot exhaust the NAS stream's budget or the other
  way round.
- Emits `event: ready` once established (the client fetches on that, closing the
  connect gap), then one `data:` frame per published batch, then a `:` heartbeat
  every 15 seconds.
- Re-resolves the session on each heartbeat and ends the stream if it is gone or
  the user's admin flag changed.

**`internal/store/ytdl_downloads.go`** — one query, `ListYtdlUnfinished()`:
everything, both kinds, not in a terminal state.

### Wire contract

See [contracts/ytdl-stream.md](./contracts/ytdl-stream.md).

### Client

**`src/services/api.ts`** — `streamYtdl(onEvent, signal)`, the same hand-rolled
SSE reader as `streamTasks`, reusing `parseSSEFrame`.

**`src/composables/useYtdl.ts`** — connect / fallback / backoff exactly as
`useTasks` does it, plus the merge:

- known id → replace it;
- `created` with no `parentId` → insert at the top (the list is newest-first);
- anything else unknown → ignore (FR-004a);
- `removed` → drop it.

An items map lives here too, so the group sheet reads from the same live state
rather than re-fetching (FR-003).

**`src/components/YtdlGroupModal.vue`** — read items from the composable instead
of its own 5-second refresh.

## Post-Design Constitution Re-check

Unchanged: PASS on every principle, no amendment, no checklist. The one design
decision that could have touched Principle III — how a long-lived connection
proves it still may see what it is sent — is answered by re-checking rather than
by trusting the connect.

## Phase 2 — Tasks

See [tasks.md](./tasks.md).
