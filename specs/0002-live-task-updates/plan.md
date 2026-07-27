# Implementation Plan: Live task updates, task detail view, and download failure reasons

**Branch**: `feat/0002-live-task-updates` | **Date**: 2026-07-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0002-live-task-updates/spec.md`

## Summary

Make the Tasks tab feel live and explain failures, without breaking the stateless-proxy
boundary. Three slices:

1. **Live updates** — a new `GET /v1/tasks/stream` Server-Sent Events endpoint. The server
   holds the request open, polls the **existing** allowlisted DSM task-list (`SYNO.DownloadStation.Task`
   `list`) about once per second, and pushes each `{tasks, stats}` snapshot as an SSE `data:` event,
   with a `:` heartbeat every ~15s so an idle stream survives the operator's 60s reverse-proxy read
   timeout. The client consumes it via a `fetch` ReadableStream (so the `sid` rides in `X-Syno-Sid`,
   never the URL) while the Tasks view is mounted and the tab is visible, and **falls back to the
   existing 3s polling** — retrying the stream with backoff — whenever the stream is unavailable or
   drops. DSM session expiry ends the stream as HTTP 401, reusing the existing return-to-login flow.
2. **Failure reason** — capture DSM's `status_extra.error_detail` in `internal/syno` and expose it as
   a new `errorDetail` field on the flat `Task` DTO (returned by both `/v1/tasks` and the stream). A
   pure client module maps known detail keywords → friendly text with a generic fallback.
3. **Task detail view** — tapping a task row opens a stock-Ionic `TaskDetailModal` bound to the same
   reactive task, showing the fuller field set (incl. the failure reason), live-updating and tolerant
   of the task disappearing.

No new DSM API is added to the allowlist; the server persists nothing; the `sid` is held only for the
life of a connection.

## Technical Context

**Language/Version**: Go 1.26 (server); TypeScript 5 / Vue 3 + Ionic 8 (client); Node 22 tooling.

**Primary Dependencies**: Server — Go **stdlib only** (`net/http`, `http.Flusher`); **zero third-party
deps** (constitution). This is the deciding reason SSE was chosen over WebSocket (stdlib has no WS).
Client — Ionic Vue components + the browser `fetch`/`ReadableStream` + `TextDecoder` (no new npm dep).

**Storage**: None on the server (stateless proxy). Client keeps session/settings in IndexedDB as
today; **task state is not persisted** — the stream is only a fresher re-sync transport.

**Testing**: Server — `go test` with a fake `syno.Client` (handler tests) and an `httptest` fake DSM
(client tests); `synomock` extended for deterministic streaming/error fixtures. Client — vitest for
the pure error-reason mapping module (on the coverage allowlist); Playwright e2e under `e2e/` for the
user-visible behaviors.

**Target Platform**: Installable PWA (mobile-first) served by `synodl` on one origin; deployed to the
operator's k3s cluster behind a TLS-terminating Synology reverse proxy (HTTP to the node's `:80`).

**Project Type**: Web (single Go service serving the built Vue/Ionic PWA + typed `/v1` API).

**Performance Goals**: A NAS-side change is reflected in the UI within ~2s (SC-001); one live
connection per client (SC-006); idle stream survives >60s (SC-006).

**Constraints**: Stateless & credential-free (no persistence; `sid` header-only, never in URL/logs);
zero server deps; bounded concurrent streams + bounded server→NAS poll cadence so the stream cannot
hammer the NAS; heartbeat ≤20s for the 60s edge read timeout; stock Ionic UI only.

**Scale/Scope**: Home-NAS scale — a handful of concurrent clients. Small change surface: ~1 new server
handler + a bound/limiter, a `status_extra` field through `internal/syno`, `synomock` fixtures, one
client composable rewrite, one pure mapping module, `TaskItem` tweak, one new modal, ~4 e2e specs.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Verdict |
|---|---|---|
| I. Spec-Driven Development | Built from spec 0002 via the pipeline. | ✅ PASS |
| II. Test-Driven Development | tasks.md will order failing tests before impl; server proxy/handler logic gets unit tests; user-facing behavior gets e2e; coverage floors ratchet, never regress. | ✅ PASS (enforced in Phase 2) |
| III. Stateless, Credential-Free Proxy (NON-NEGOTIABLE) | No persistence; `sid` in `X-Syno-Sid` header only (fetch stream, not `EventSource`) — no query-string smuggling; only the existing allowlisted task-list call is forwarded; no new `SYNO.*` API; logs carry route+outcome+stream-lifecycle only, never sid/creds/OTP/URIs. Spec has the Credential-Safety Impact section. | ✅ PASS |
| IV. Offline-First Client Data | Task state still not persisted; no object store added → no `DB_VERSION` bump. | ✅ PASS |
| V. Quality Gates | Definition of done = all CI gates green (`npm run build`, `go build/vet/test`, vitest+floors, e2e). Commit subjects are user-facing release-note copy. | ✅ PASS (enforced at merge) |
| VI. Ionic-First UI | Detail view built from stock `ion-modal`/`ion-list`/`ion-item`, themed only with `--app-*` tokens; no hand-rolled widgets. | ✅ PASS |
| VII. Zero server deps | SSE via stdlib `http.Flusher`; no library added. | ✅ PASS |

No violations → **Complexity Tracking table intentionally empty.**

## Project Structure

### Documentation (this feature)

```text
specs/0002-live-task-updates/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions + rationale
├── data-model.md        # Phase 1 — Task/errorDetail, snapshot, SSE event
├── quickstart.md        # Phase 1 — how to run & test locally
├── contracts/
│   ├── tasks-stream.md  # GET /v1/tasks/stream SSE contract
│   ├── tasks-list.md    # GET /v1/tasks — added errorDetail field
│   └── synomock-control.md # /__mock control additions (error + mutation)
├── checklists/
│   └── requirements.md  # from /speckit-specify (passing)
└── tasks.md             # Phase 2 — /speckit-tasks output (not created here)
```

### Source Code (repository root)

```text
server/
├── cmd/synomock/main.go              # wire new /__mock controls (error task, state mutation)
└── internal/
    ├── api/
    │   ├── router.go                 # + mux.Handle("GET /v1/tasks/stream", ...)
    │   ├── tasks_stream.go           # NEW: SSE handler (http.Flusher, heartbeat, ctx cancel)
    │   ├── tasks_stream_test.go      # NEW: handler unit tests (fake syno.Client)
    │   ├── stream_limit.go           # NEW: concurrent-stream bound (mirrors ratelimit.go)
    │   └── stream_limit_test.go      # NEW
    ├── syno/
    │   ├── client.go                 # Task DTO: + ErrorDetail string `json:"errorDetail"`
    │   ├── http.go                   # dsmTask: + StatusExtra.ErrorDetail; map it through
    │   └── http_test.go              # extend: error_detail parsed from fake DSM
    └── synomock/
        └── *.go                      # emit status_extra.error_detail; time/step task state

src/ (client)
├── types/task.ts                     # Task: + errorDetail?: string
├── services/
│   ├── task-error.ts                 # NEW pure module: errorDetail → friendly text (vitest floor)
│   └── task-error.test.ts            # NEW unit tests
├── composables/useTasks.ts           # SSE via fetch-stream + polling fallback + reconnect backoff
├── services/api.ts                   # + streamTasks() helper (fetch ReadableStream, X-Syno-Sid)
├── components/
│   ├── TaskItem.vue                  # show failure reason on errored rows; emit @open
│   └── TaskDetailModal.vue           # NEW stock-Ionic detail modal
└── views/tabs/TasksPage.vue          # open detail on row tap; pass live task

e2e/
└── live-updates.spec.ts (+ siblings) # live update, error reason, detail view, fallback
```

**Structure Decision**: Existing web layout (one Go service + Vue client). New server code follows the
established one-handler-file-plus-sibling-`_test.go` pattern in `internal/api`, depending on the small
`syno.Client` interface. Client follows the composable + pure-service-module pattern, `@/` alias, stock
Ionic. No structural changes.

## Complexity Tracking

> No constitution violations — nothing to justify.
