# Feature Specification: YouTube downloads update as they happen

**Feature Branch**: `feat/1038-ytdl-live-updates`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: From use: "the status of the downloads seem to be more like a pull
flow than a sse flow, I see the sub tasks under a playlist getting refreshed
every 5 seconds or so."

## Overview

NAS tasks stream. YouTube downloads poll — every client asks for the whole list
every five seconds whether anything changed or not, and a group's items are
fetched separately on top of that.

Spec 0013 chose that deliberately and said why: the existing stream is built
around the NAS session and per-user task snapshots, YouTube downloads change
state far more slowly than a NAS transfer, and a five-second poll was judged
adequate for a progress bar. It recorded the decision as revisitable "only if it
proves not to be". It has: the polling is visible in use, and it is the reason a
rendering fault (spec 2024) read as a flicker rather than as a stale row.

This makes YouTube downloads live, the way NAS tasks already are.

There is one honest ceiling to state up front. The server's own picture of a
download only advances when the reconciler runs — every three seconds — because
that is what reads a worker's output. Streaming removes the client's repeated
asking and delivers a change when it happens; it does not make the underlying
state finer-grained. Making it feel instant means the reconciler announcing
changes rather than a stream re-reading on its own clock.

## User Scenarios & Testing

### User Story 1 - The list updates itself (Priority: P1)

Someone watching the Tasks list sees a download's state and progress change as it
changes, without the app asking for the whole list over and over.

**Why this priority**: it is the request, and it removes the repeated work that
made a rendering fault look like a refresh.

**Independent Test**: hold the list open, drive a download through its states,
and confirm the row follows without a repeated list fetch.

**Acceptance Scenarios**:

1. **Given** the Tasks list is open, **When** a download changes state, **Then**
   the row updates without the client re-fetching the list.
2. **Given** nothing is changing, **When** several seconds pass, **Then** no list
   request is made.
3. **Given** the stream cannot be established, **When** the list is shown,
   **Then** it still updates by asking — a fallback, not a blank screen.
4. **Given** the stream drops, **When** the connection is lost, **Then** it is
   re-established without the reader doing anything.

---

### User Story 2 - An open playlist updates itself (Priority: P1)

Someone watching a playlist's tracks sees each one progress without the sheet
re-reading its contents.

**Why this priority**: this is where the polling was actually noticed, and where
a full re-read is most wasteful — a channel has no ceiling on how many tracks it
holds.

**Acceptance Scenarios**:

1. **Given** an open playlist, **When** one of its tracks changes, **Then** that
   row updates and the others are untouched.
2. **Given** an open playlist of many tracks, **When** one changes, **Then** the
   update carries that track, not the whole list.

---

### User Story 3 - The instance is not made to work harder (Priority: P2)

Several people watching does not multiply what the server or the cluster does.

**Acceptance Scenarios**:

1. **Given** several viewers, **When** they all watch, **Then** the orchestrator
   is asked no more often than with one viewer.
2. **Given** more viewers than the instance will carry, **When** another
   connects, **Then** they fall back to asking rather than being refused
   outright.

---

### Edge Cases

- **A reverse proxy that closes an idle connection.** The operator's Synology
  proxy closes a read after 60 seconds; the existing stream carries a heartbeat
  for exactly this and so must this one.
- **A very large group.** A channel can hold thousands of tracks. Whatever is
  sent when one changes must not be proportional to how many exist.
- **A page the reader has not loaded.** History is unbounded and the list is
  paged; an update about a row the client has never seen is not an instruction to
  load it.
- **A download that is dismissed elsewhere.** Removal has to reach a watching
  client too, or a row lingers that no longer exists.
- **Ownership.** A stream carries only what its reader may see, and an admin's
  stream carries everyone's — the same rule the list already applies.

## Requirements

### Functional Requirements

- **FR-001**: A watching client MUST be told when a download it can see changes,
  without asking.
- **FR-002**: An update MUST NOT require the client to re-read the list.
- **FR-003**: A group's items MUST update in the same way, without the open sheet
  re-reading its contents.
- **FR-004**: An update MUST carry only the downloads that changed. Its size MUST
  NOT grow with how many downloads exist — a channel of five hundred tracks sends
  the same small event as a single download.
- **FR-004a**: A client MUST merge an update into what it already holds, and MUST
  ignore an update about a download it has not loaded. History is unbounded and
  the list is paged, so an update is not an instruction to fetch a page.
- **FR-004b**: A client that has missed updates — a dropped connection, a long
  sleep — MUST be able to recover a correct picture without the stream having to
  replay anything.
- **FR-005**: The stream MUST carry the session in a header, never in the URL,
  as the existing stream does (Principle III).
- **FR-006**: The stream MUST carry only downloads its reader may see, with the
  admin exception the list already applies.
- **FR-007**: The stream MUST survive an idle connection through the operator's
  reverse proxy.
- **FR-008**: A client that cannot stream MUST fall back to asking, and MUST NOT
  show an empty or stale list instead.
- **FR-009**: A dropped stream MUST be re-established without user action.
- **FR-010**: Watching MUST NOT increase how often the orchestrator is asked,
  however many viewers there are.
- **FR-011**: The number of concurrent streams MUST be bounded, as the existing
  stream is, and exceeding the bound MUST degrade to asking rather than failing.
- **FR-012**: A change MUST be announced by whatever made it, at the moment it is
  made, rather than discovered by a stream re-reading on its own interval. The
  reconciler already knows the instant something moves — it is what reads the
  worker output — so an update leaves the server as soon as the server knows.
- **FR-012a**: Announcing MUST NOT block the reconciler. A slow or stuck reader
  must not be able to hold up the loop that admits work and captures outcomes.
- **FR-012b**: A reader that cannot keep up MUST be dropped rather than allowed
  to accumulate an unbounded backlog; it reconnects and recovers under FR-004b.
- **FR-013**: Removal MUST reach a watching client, so a dismissed download does
  not linger.

### Key Entities

- **Download update**: what changed about one download — its state, progress,
  outcome, or the fact that it is gone.
- **Stream subscription**: one reader's live connection, scoped to what that
  reader may see and bounded with the others.

## Success Criteria

### Measurable Outcomes

- **SC-001**: With the list open and nothing changing, no request is made for
  several minutes beyond what keeps the connection alive.
- **SC-002**: A state change leaves the server as soon as the server knows it,
  rather than waiting for a re-read; a watching client sees it within a second.
- **SC-002a**: Ten viewers cause no more database reads than one, because a
  change is announced once and fanned out rather than discovered per connection.
- **SC-003**: With an open playlist of hundreds of tracks, one track changing
  sends an update that does not grow with the number of tracks.
- **SC-004**: Ten viewers cause no more orchestrator calls than one.
- **SC-005**: With streaming unavailable, the list still updates.

## Credential-Safety Impact

- **No new stored data, no new worker input, no new permission.** This changes
  how what is already computed reaches a client.
- **The session stays in a header.** The existing stream deliberately uses fetch
  and a stream reader rather than `EventSource`, because `EventSource` cannot
  carry a header and would put the session in the URL. This must do the same.
- **Ownership is enforced per connection**, and re-checked rather than captured
  once — a long-lived connection outlives the moment it was opened.
- **Nothing new reaches a log.** A stream carries the same values the list does,
  under the same rule about what may not be logged.

## Clarifications

### Session 2026-09-09

- Q: When a download changes, what should the stream send? → A: Only the rows
  that changed. A whole snapshot would mirror the NAS task stream and keep the
  client trivial, but the payload would grow with history — which is unbounded —
  and the list is paged, so "a snapshot of what?" has no good answer. Deltas keep
  a five-hundred-track channel sending the same small event as a single download.
  Carried into FR-004, FR-004a and FR-004b.
- Q: How should a change reach a watching client? → A: The reconciler announces
  it. It is already the thing that knows the moment something moves, so an update
  leaves the server as soon as the server knows, and adding viewers costs
  nothing. A per-connection re-read would be simpler and needs no subscriber
  registry, but it is still polling — moved server-side — and it multiplies
  database reads by the number of watchers. Carried into FR-012, FR-012a,
  FR-012b and SC-002a.
- Deferred with a documented default rather than asked (marker limit): the
  stream is a NEW endpoint rather than an extension of the NAS task stream, and
  a client with no stream falls back to the existing poll. Both are in
  Assumptions and revisitable in the plan.

## Assumptions

- The reconciler remains the only thing that reads worker output; streaming
  changes delivery, not derivation.
- Progress remains a cached reading held in memory, not stored — Principle III is
  unchanged by this.
- **The stream is a new endpoint**, not an extension of the NAS one. The two have
  different cadences, different failure modes, and one is tied to the NAS
  session; sharing a connection is a bigger change than this earns.
- **The poll remains as a fallback**, not as the normal path. It is what a client
  uses when the stream cannot be established or the instance is at its limit.
- The existing NAS task stream is left alone. Sharing one connection between two
  systems is a bigger change than this earns, and the two have different
  cadences and different failure modes.
