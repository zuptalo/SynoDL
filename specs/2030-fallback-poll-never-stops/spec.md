# Feature Specification: The fallback poll stops when the stream goes live

**Feature Branch**: `fix/2030-fallback-poll-never-stops`

**Created**: 2026-09-15

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Observed in production: `/v1/ytdl` was requested 1577 times in 2h12m
at a fixed 5s cadence, with the longest gap between requests being 5.9s — 157 of
them during a single 790-second OPEN stream.

## Overview

Spec 1038 replaced a five-second poll with a server-push stream, keeping the poll
as a fallback "used ONLY while the stream is down". In production the fallback
runs alongside the stream instead, so the NAS is asked for the list every five
seconds forever while a perfectly healthy stream delivers the same data.

The stream is not at fault. Every connection in the observed window returned 200,
and each `ytdl`/`tasks` pair ended within a sub-millisecond of its twin —
one client aborting both on `visibilitychange`, which is the designed behaviour
for a backgrounded app. The server's SSE headers, flushing and heartbeat are all
correct, and nothing in the operator's reverse proxy interferes.

What fails is stopping the poll. The tick clears its own handle BEFORE awaiting
the request, and `stopPolling` only acts on a non-null handle:

```
const tick = async () => {
  pollTimer = null;                       // the handle stop() looks for is gone
  if (watchers === 0) return;
  if (visible) await refresh();           // ← stopPolling() lands in here
  if (watchers > 0) pollTimer = setTimeout(tick, POLL_MS);   // re-arms anyway
};
```

So a `stopPolling()` arriving while the poll's own request is in flight is
silently lost, and the tick re-arms. The loop is then immortal. Nothing heals it,
because `stopPolling()` is called once per CONNECTION (in the stream's `ready`
handler), not once per snapshot — and a long-lived stream never reconnects.

`useTasks` has the same shape and the same hole; it simply has not been observed
hitting it.

## User Scenarios & Testing

### User Story 1 - A live stream means no polling (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the fallback poll is running, **When** the stream goes live, **Then**
   the poll stops.
2. **Given** the poll's own request is IN FLIGHT, **When** the stream goes live
   in that window, **Then** the poll still stops.
3. **Given** the stream then drops, **When** it does, **Then** the poll resumes.

---

### User Story 2 - The app is unchanged in every other respect (Priority: P2)

**Acceptance Scenarios**:

1. **Given** no stream, **When** the list is on screen, **Then** it still polls
   at the same cadence.
2. **Given** the view is hidden or unmounted, **When** it is, **Then** nothing
   polls.

---

### Edge Cases

- **Stop called before the first tick.** Must cancel the pending timer, as today.
- **Start called twice.** Must not run two loops.
- **Stop then start.** Must resume cleanly rather than stay stopped.

## Requirements

- **FR-001**: Stopping the fallback poll MUST stop it, including while its own
  request is in flight.
- **FR-002**: Whether the loop continues MUST be decided by explicit state, not
  by whether a timer handle happens to be set — the handle is not a reliable
  record of intent, since the tick clears it.
- **FR-003**: Both the downloads list and the NAS task list MUST use the same
  implementation, so the invariant cannot hold in one and not the other.
- **FR-004**: Cadence, visibility behaviour and unmount behaviour MUST be
  unchanged.

## Success Criteria

- **SC-001**: A stop issued during an in-flight tick results in no further
  requests.
- **SC-002**: With a live stream, zero fallback requests are made.

## Credential-Safety Impact

- **None.** Client-side scheduling only; no new endpoint, no new state, no change
  to what is sent or logged.

## Assumptions

- The loop is worth extracting to its own pure module rather than fixed twice in
  place. It is the kind of thing that reads as correct in both copies and is
  wrong in both, and a shared module is the only way the unit test covers the
  code that actually runs.
