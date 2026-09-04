# Feature Specification: Live task updates, task detail view, and download failure reasons

**Feature Branch**: `feat/0002-live-task-updates`

**Created**: 2026-07-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Live task updates via Server-Sent Events, a task detail view, and surfacing the DSM error reason."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See downloads update live, without lag (Priority: P1)

As someone watching my downloads, I open the Tasks tab and the list stays current on its own — progress, speeds, and status change in front of me within a second or two of the NAS changing, without me pulling to refresh or the list visibly "ticking" on a timer.

**Why this priority**: This is the point of the feature. The app already shows the right data; it just doesn't feel alive. A responsive, self-updating list is the difference between "a status page I have to poke" and "a live view of my NAS."

**Independent Test**: With a task actively downloading on the NAS (mock DSM), open the Tasks tab and, without touching anything, observe progress and the global speed indicators advance within ~2s of the NAS state changing. Fully delivers value on its own.

**Acceptance Scenarios**:

1. **Given** a task is downloading on the NAS, **When** its progress or speed changes, **Then** the row and the global speed indicators reflect the new values within ~2 seconds with no user action.
2. **Given** a task changes status on the NAS (e.g. downloading → finished, or a new task appears / one is removed), **When** that happens, **Then** the list adds/removes/updates that row within ~2 seconds without a manual refresh.
3. **Given** I switch the app to the background (tab hidden), **When** it is not visible, **Then** the app stops streaming so it does not keep the NAS busy, and **When** I return, **Then** it resumes and shows current state immediately.

---

### User Story 2 - Understand why a download failed (Priority: P1)

As someone who sees a download in the Error state, I want to know *why* it failed — a broken link, a full disk, a duplicate, a missing destination — instead of just the word "Error", so I know whether to retry, fix something, or give up.

**Why this priority**: A failure with no reason is a dead end. The NAS already knows the cause; today the app throws it away. This is small to surface and high-value — the operator hit it on their very first real task.

**Independent Test**: With the mock DSM emitting a task in the error state carrying an error detail, open the app and confirm the row (and detail view) show a specific, human-readable reason rather than only "Error".

**Acceptance Scenarios**:

1. **Given** a task is in the error state with a known failure detail, **When** I view the list, **Then** I see a specific human-readable reason associated with that task (e.g. "Broken link", "Destination not found", "Torrent already added").
2. **Given** a task is in the error state with an unknown or absent failure detail, **When** I view it, **Then** I see a graceful generic message and the app does not misbehave.

---

### User Story 3 - See a task's full details (Priority: P2)

As someone triaging a download, I tap a task row and get a detail view with everything about it — full (untruncated) name, status, the failure reason when errored, destination folder, when it was added, size/downloaded/uploaded, peers/seeders, and current speeds.

**Why this priority**: The list is intentionally compact and truncates long names; there is currently nowhere to see the full picture, and it is the natural home for the failure reason from User Story 2.

**Independent Test**: Tap any task and confirm a detail view opens showing the fuller set of fields for that task, built from stock Ionic components.

**Acceptance Scenarios**:

1. **Given** the task list is showing, **When** I tap a task row, **Then** a detail view opens showing that task's full name, status, destination, created time, size/downloaded/uploaded, peers/seeders, and current speeds.
2. **Given** the detail view is open for an errored task, **When** I read it, **Then** the failure reason is shown.
3. **Given** the detail view is open and the task keeps updating live, **When** its values change on the NAS, **Then** the open detail view reflects the latest values.
4. **Given** the detail view is open, **When** the task finishes or is removed on the NAS, **Then** the view handles this gracefully (updates or closes) without error.

---

### User Story 4 - Stay working when the live connection drops (Priority: P2)

As someone on flaky Wi‑Fi or behind a proxy that closes idle connections, I want the app to keep showing current downloads even if the live connection breaks — it should quietly recover, and I should never be left staring at a frozen list.

**Why this priority**: A "live" feature that fails hard on a dropped connection is worse than the old timer. Graceful degradation is what makes it trustworthy on mobile.

**Independent Test**: Interrupt the live connection while the Tasks tab is open and confirm the list keeps updating (via the fallback path) and the live connection re-establishes on its own once possible — with no reload.

**Acceptance Scenarios**:

1. **Given** the live connection cannot be established, **When** I open the Tasks tab, **Then** the list still loads and keeps updating on the existing periodic basis.
2. **Given** a working live connection drops, **When** it breaks, **Then** the app falls back to periodic updates and automatically retries the live connection (with backoff) without user action.
3. **Given** the live connection is idle because nothing is changing on the NAS, **When** more than 60 seconds pass, **Then** the connection stays alive (is not dropped by an intermediary read timeout).
4. **Given** the session expires while connected, **When** the NAS reports the session invalid, **Then** the app returns to the login screen (the existing session-expiry behavior), not a silent stall.

---

### Edge Cases

- **Idle stream vs. proxy timeout**: nothing changes on the NAS for over a minute — the connection must survive the operator's reverse-proxy 60s read timeout (via a periodic heartbeat).
- **Session expiry mid-stream**: DSM reports the session expired/invalid — the stream must end and be mapped to the app's existing 401 → return-to-login flow.
- **Backgrounded app**: a hidden tab must not hold an open stream or keep polling the NAS.
- **Many clients / reconnect storms**: concurrent live connections and the server→NAS poll cadence must be bounded so the feature cannot be used (accidentally or otherwise) to hammer the NAS.
- **Unknown/absent failure detail**: an errored task with a code we do not recognize (or none) degrades to a generic message.
- **Task disappears while its detail view is open**: must not error.
- **Environment without live support**: if the streaming path is unavailable end-to-end, the app must still work on the fallback path.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a way for the client to receive a continuous stream of task-list + global-stats snapshots over a single long-lived connection, authenticated by the session id carried in the request header (never in the URL or query string).
- **FR-002**: The server MUST source those snapshots by polling ONLY the already-allowlisted DSM task-list capability on a bounded cadence (target ~1 second); this feature MUST NOT add any new DSM API to the allowlist.
- **FR-003**: The server MUST send a heartbeat at least every 20 seconds so an otherwise-idle stream survives intermediary read timeouts (the operator's reverse proxy closes idle reads at 60 seconds).
- **FR-004**: The server MUST end the stream and signal session-expiry (using the app's existing expired-session semantics) when the NAS reports the session invalid/expired, so the client returns to login.
- **FR-005**: The server MUST bound the number of concurrent streams and the server→NAS poll cadence to protect the NAS from excessive load (mirroring the spirit of the existing per-IP login rate limit); excess connections MUST be shed gracefully rather than overrunning the NAS.
- **FR-006**: The streaming path MUST persist nothing; the session id is held only for the lifetime of the connection; the session id, credentials, OTP codes, and full task URIs MUST NOT appear in logs, errors, or metrics.
- **FR-007**: The client MUST consume the live stream while the Tasks view is active and the tab is visible, updating the list, each task's progress/speeds, and the global speeds without any manual refresh.
- **FR-008**: The client MUST stop consuming (close the stream) when the tab is hidden or the view is left, and resume when it becomes active/visible again.
- **FR-009**: The client MUST automatically fall back to the existing periodic update behavior if the live stream fails to start, errors, or disconnects, and MUST attempt to re-establish the stream with backoff — the user MUST never be left with a frozen list and no recovery.
- **FR-010**: The server MUST capture the NAS-provided failure detail for errored tasks and include it on the task record returned by BOTH the existing task list and the live stream.
- **FR-011**: The client MUST present a human-readable reason for errored tasks, mapping known failure details to friendly text and degrading to a generic message for unknown/absent details.
- **FR-012**: Users MUST be able to open a task's detail view by tapping its row; the detail view MUST show the full (untruncated) name, status, the failure reason when errored, destination, created time, size/downloaded/uploaded, peers/seeders, and current speeds, composed from stock Ionic components.
- **FR-013**: While open, the detail view MUST reflect the latest live values for its task and MUST handle the task finishing or disappearing without error.
- **FR-014**: This feature MUST NOT change which existing fields are shown in the list, nor the existing filter/sort/search behavior; it changes only update freshness/transport and adds the failure reason and the detail view.

### Key Entities *(include if feature involves data)*

- **Task update snapshot**: the full current set of tasks plus the global download/upload speed totals at a moment in time. Already exists as the task-list response; this feature additionally delivers it continuously.
- **Task failure reason**: a human-readable explanation for an errored task, derived from the NAS-provided failure detail; has a known-code → friendly-text mapping and a generic fallback.
- **Task (detail presentation)**: the same task record shown in fuller form, including fields the compact list omits.

## Credential-Safety Impact *(constitution-required)*

- **What crosses the proxy**: the session id in the request header on the stream connection — the same credential already sent on every call. No password or OTP is involved in this feature; no new secret crosses the proxy.
- **What is forwarded to the NAS**: only the existing allowlisted task-list poll, over the operator's `SYNO_URL`. No new DSM APIs; no client-supplied targets; no query-string smuggling (the sid stays in the header).
- **What could appear in logs/errors**: route, outcome, and stream lifecycle (open/close/heartbeat/shed) only. The sid, credentials, OTP codes, and full task URIs are never logged; the added failure detail is a DSM status code/keyword, not a secret.
- **Why nothing sensitive is retained**: the server has no store; the sid lives only for the connection and is dropped when it closes; task state is not persisted (Principle IV); a restart loses nothing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A change to a task on the NAS is reflected in the UI within ~2 seconds with no manual refresh.
- **SC-002**: While a download is active, its progress and the global speed indicators visibly advance without any user interaction.
- **SC-003**: Every errored download shows a specific, human-readable reason (not just "Error") for each failure detail the reference app surfaces; unknown details show a graceful generic message.
- **SC-004**: Tapping any task opens its detail view within ~1 second, showing the fuller field set.
- **SC-005**: If the live connection drops, the list keeps updating within the previous update interval and the live connection re-establishes automatically once possible — the user never has to reload.
- **SC-006**: A client holds at most one live connection at a time, and an idle stream (no task changes) stays connected beyond 60 seconds without being dropped.
- **SC-007**: The change introduces no persisted server state and no new DSM APIs beyond the existing task-list capability.

## Assumptions

- Intermediary read timeouts are on the order of 60 seconds (the operator's Synology reverse proxy is configured at 60s), so a ≤20s heartbeat is sufficient to keep an idle stream alive.
- A ~1 second server-side poll cadence is adequate for a "live" feel. DSM's own long-polling capability (`SYNO.DownloadStation2.Task.List.Polling`, confirmed present on the operator's DSM 7) is a **future proxy-internal optimization tracked as a separate follow-up spec** and is explicitly out of scope here; because the client only ever consumes the stream, adopting it later requires no client-facing change.
- The mock DSM (`synomock`) can be extended so tests can (a) emit a task in the error state carrying a failure detail and (b) change task state over time (or via a control endpoint), making live updates and the failure reason end-to-end testable without real hardware.
- Known failure details and their friendly text mirror the reference app's set; unrecognized details degrade to a generic message.
- No IndexedDB schema change is required (task state is not persisted client-side beyond display caches); if one were ever needed it would bump `DB_VERSION` — not expected here.

## Out of Scope

- **WebSocket transport** — the server has a zero-third-party-dependency rule and the standard library has no WebSocket; Server-Sent Events over a fetch stream is the deliberate choice.
- **Upstream DSM long-polling** via `SYNO.DownloadStation2.Task.List.Polling` — a planned **separate follow-up spec** to make the proxy's NAS-side updates near-instant; no client-facing change since the client always consumes the stream.
- **Search, Browser, and RSS tabs** — separate roadmap specs.
- **Per-task push from DSM** — DSM exposes no push feed for Download Station; the proxy polls regardless.

## Clarifications

### Session 2026-07-27

- Q: Which transport for client↔server live updates? → A: **Server-Sent Events** over a `fetch` stream (so the sid rides in the `X-Syno-Sid` header, not the URL). WebSocket rejected to preserve the server's zero-dependency rule.
- Q: How should the proxy get fresh data from the NAS, given DSM offers long-polling (`…Task.List.Polling`) but no push? → A: **Fast-poll the existing v1 task-list now** (approach "A"); adopt DSM long-polling later as a **separate follow-up spec** (approach "B"). The client contract (SSE) is identical either way.
- Q: Scope of this spec? → A: live updates + failure reason + task detail view, together, as one planned spec.
