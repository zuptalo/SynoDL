# Research: Live task updates, failure reasons, detail view

Phase 0 decisions. All spec "NEEDS CLARIFICATION" were pre-resolved by the operator (see spec
Clarifications); this records the technical rationale and the alternatives weighed.

## D1 — Client↔server transport: SSE over a fetch stream

- **Decision**: Server-Sent Events on `GET /v1/tasks/stream`, consumed on the client with
  `fetch()` + `response.body.getReader()` + `TextDecoder` (NOT the `EventSource` API).
- **Rationale**: Updates are one-way (server→client), which is exactly SSE's shape. Using `fetch`
  instead of `EventSource` lets the `sid` travel in the `X-Syno-Sid` **request header**, satisfying
  Constitution III ("no query-string smuggling"); `EventSource` cannot set headers and would force the
  sid into the URL. SSE needs only stdlib `http.Flusher` on the server → honors the zero-dependency
  rule (Principle VII).
- **Alternatives**: **WebSocket** — bidirectional (unneeded) and, decisively, absent from the Go
  stdlib, so it would require a third-party library or a hand-rolled RFC 6455 framing layer; rejected
  to keep zero deps. **EventSource** — simplest client, but sid-in-URL is disqualifying. **Keep
  client polling only** — no new endpoint, but can't feel live without hammering the NAS from every
  client; rejected.

## D2 — Server data source: fast-poll the existing v1 list (approach "A")

- **Decision**: The stream handler loops on a ~1s ticker calling the existing
  `syno.Client.ListTasks` (`SYNO.DownloadStation.Task` `list`, already allowlisted), and writes a
  snapshot whenever it has fresh data. No new DSM API.
- **Rationale**: ~1s is well within the ~2s liveness target (SC-001) and reuses tested code and the
  existing allowlist — lowest risk. Because the **client contract is SSE regardless**, the server can
  later be upgraded to DSM long-polling with no client change.
- **Alternatives**: **DSM `SYNO.DownloadStation2.Task.List.Polling`** (confirmed present on the
  operator's DSM 7) gives near-instant, native-app-fidelity updates but pulls in the whole
  DownloadStation2 API family (`entry.cgi`, JSON params, different field shapes) and an allowlist
  expansion + field QA — deferred to a **separate follow-up spec (approach "B")** per the operator.
- **Optimization (non-blocking)**: the handler MAY suppress emitting a snapshot identical to the last
  one it sent on that connection (cheap equality on the marshaled payload), so idle streams cost only
  the heartbeat. Not required for correctness.

## D3 — Heartbeat & connection lifetime

- **Decision**: Emit an SSE comment line `:\n\n` every ~15s when no data event was sent. Set
  `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`,
  `X-Accel-Buffering: no`; flush after every write.
- **Rationale**: The operator's Synology reverse proxy has a 60s read timeout; ≤20s heartbeat keeps
  the connection alive (SC-006). `X-Accel-Buffering: no` and explicit flushing defeat intermediary
  buffering so events arrive promptly.
- **Alternatives**: rely on TCP keepalive — insufficient (proxy read timeout is application-level).

## D4 — Protecting the NAS (bounded streams + cadence)

- **Decision**: A process-wide counter caps concurrent streams (small, e.g. 16); over-cap requests
  get `503` + `Retry-After`. The per-connection poll cadence is a fixed server constant (~1s), not
  client-tunable. Mirrors the spirit of the existing per-IP limiter on `POST /v1/session`
  (`internal/httpx`/`ratelimit`).
- **Rationale**: Prevents a reconnect storm or many tabs from turning into unbounded NAS polling.
  Since each client holds at most one stream (client enforces; server caps globally), NAS load is
  bounded and predictable.
- **Alternatives**: per-IP stream limit — home-NAS scale doesn't need it; a global cap is simpler and
  sufficient. Client-supplied cadence — rejected (client must not be able to speed up NAS polling).

## D5 — Session expiry on the stream

- **Decision**: When `ListTasks` returns the typed session-expired error (DSM 105/106/107/119, already
  mapped in `internal/syno`), the handler stops the loop. If nothing has been written yet it responds
  `401`; if the stream is already open it emits a terminal `event: error` with a 401-equivalent
  payload and closes. The client treats stream-close-with-auth-error identically to today's 401 →
  dispatch session-expired → router bounces to login.
- **Rationale**: Reuses the one existing session-expiry path; no new client bounce logic.

## D6 — Failure reason: where `error_detail` lives & how it's mapped

- **Decision**: DSM's `SYNO.DownloadStation.Task` `list` task object carries `status_extra`
  (top-level sibling of `status`/`additional`) with a string `error_detail` keyword when the task is
  in a problem state. Add `StatusExtra.ErrorDetail` to `dsmTask` and surface it as `Task.ErrorDetail`
  (`json:"errorDetail"`). A pure client module `task-error.ts` maps known keywords → friendly text.
- **Known keyword set** (mirrors the reference app; unknown/empty → generic "Download failed"):
  `broken_link` → "Broken link", `destination_not_exist` → "Destination folder no longer exists",
  `destination_denied` → "No permission for the destination folder", `disk_full` → "The disk is full",
  `quota_reached` → "Storage quota reached", `timeout` → "The connection timed out",
  `exceed_max_fs_size` → "File exceeds the filesystem's maximum size",
  `exceed_max_temp_size`/`exceed_max_dest_size` → size-limit variants,
  `name_too_long` → "The file name is too long", `torrent_duplicate` → "Already added",
  `required_premium` → "Requires a premium account", `try_it_later` → "Try again later",
  `encryption`/`decryption` → "Encryption error", `missing_python` → "Python is required on the NAS",
  `private_video` → "The video is private", `ftp_encryption_not_supported`,
  `extract_failed`/`extract_failed_disk_full`/`extract_failed_invalid_archive`/
  `extract_failed_quota_reached`/`extract_failed_wrong_password` → extraction-failure variants,
  `unknown` → generic.
- **QA (sid-safe)**: confirm the exact keyword set/casing against the real NAS (a DSM 7 NAS with a
  live errored task) **without** placing a `sid` in any transcript or log — operator runs the check,
  or it's run through a redacting path. Tracked as a task; the generic fallback makes the feature
  correct even before confirmation.
- **Rationale**: Keeps DSM version quirks in `internal/syno` and the human copy in one tested pure
  module (Principle VI/coverage floor).

## D7 — Client fallback & reconnect

- **Decision**: `useTasks` tries the stream first while mounted + visible. On open failure or
  mid-stream error/close (non-auth), it (a) immediately does a one-shot `refresh()` and starts the
  existing 3s poll, and (b) schedules a stream reconnect with capped exponential backoff (e.g.
  1s→2s→5s→10s, cap 10s). A successful reconnect stops the fallback poll. Hidden tab / unmount aborts
  the stream (`AbortController`) and clears timers. Auth errors bypass fallback → session-expiry flow.
- **Rationale**: Satisfies FR-009/SC-005 (never a frozen list; auto-recovers) and FR-008 (don't keep
  the NAS awake in the background). Keeps the proven poll as the safety net.
- **Alternatives**: stream-only (no fallback) — fails hard on flaky networks; rejected.

## D8 — Detail view

- **Decision**: New `TaskDetailModal.vue` (stock `ion-modal` + `ion-list`/`ion-item`), opened from
  `TasksPage` on row tap, receiving the task **id**; it reads the live task from the same reactive
  `visible`/`tasks` source so it updates in place, and closes/greys gracefully if the id vanishes.
  `TaskItem` gains an `@open` emit on tap (the sliding options for pause/resume/delete stay).
- **Rationale**: Binding by id to the live collection (not a snapshot copy) gives FR-013 live updates
  for free. Stock Ionic satisfies Principle VI.
