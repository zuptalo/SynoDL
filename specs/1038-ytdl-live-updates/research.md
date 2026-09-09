# Research: YouTube downloads update as they happen

## 1. What the existing NAS stream already settled

`GET /v1/tasks/stream` (spec 0006) is the precedent, and most of its decisions
transfer unchanged:

- **`fetch` + `ReadableStream`, never `EventSource`.** `EventSource` cannot send
  a header, so the session would have to travel in the URL — where it reaches
  proxy logs and browser history. Principle III forbids it. `src/services/api.ts`
  parses SSE frames by hand for exactly this reason, and the new stream reuses
  that parser.
- **A 15-second heartbeat comment.** The operator's Synology reverse proxy closes
  a read after 60 seconds. `:\n\n` is a comment every SSE client ignores.
- **`X-Accel-Buffering: no`.** Without it nginx and the Synology proxy buffer the
  response and nothing arrives until the connection closes.
- **Bound, then shed with 503 + `Retry-After`.** An over-cap connection must cost
  nothing before it is refused, so the limiter is taken before any work.
- **Client falls back to polling and reconnects with capped backoff.**
  `useTasks` is the working shape: poll to paint, upgrade to the stream, drop
  back to polling on any transport failure, and never fall back on a 401.

**Decision**: mirror the transport wholesale, and reuse `parseSSEFrame` and the
connect/fallback/backoff shape rather than inventing a second dialect.

**Alternative considered**: extend the NAS stream to carry YouTube downloads too,
one connection for both. Rejected in the spec's Assumptions and confirmed here:
the NAS stream dies on NAS session expiry, and coupling would make a NAS
credential problem stop YouTube updates, which are unrelated to the NAS.

## 2. Where a change actually becomes known

This is the question the feature turns on, and the answer is not symmetrical
with NAS tasks.

A YouTube download's shown state has three sources:

| What | Where it lives | When it changes |
|---|---|---|
| queued / resolving / final state | the store | when something writes it |
| scheduled / downloading | the **orchestrator's job list** | when the reconciler lists jobs |
| progress percentage | an in-memory cache | when the reconciler reads pod output |
| a group's counts | derived from its items | when an item changes |

Two of those four are only ever learned inside `reconcileYtdlOnce`. A stream that
re-read the store on its own clock would therefore still not see a download start
downloading until the reconciler noticed — it would be polling the *store*
against state the *reconciler* owns.

**Decision**: the watch step runs at the end of a reconcile cycle, with the job
list and the fresh progress cache already in hand. That is the moment the server
knows, which is what FR-012 asks for.

**Consequence worth stating plainly**: this does not make updates faster than the
reconcile interval. Three seconds is the floor, and the spec's Overview says so.
What it removes is every client asking, and the extra latency of asking on a
different clock than the one that produces the answer.

## 3. Deltas without a subscription registry per row

FR-004 wants only what changed. The reconciler does not currently compute a diff
— it acts on each concern in turn and writes as it goes, in six different places.
Threading an "announce" call through all six would work but would be six chances
to forget one, and would miss the two derived sources above entirely.

**Decision**: **fingerprint diffing over the unfinished set.** After each cycle,
project every non-final download exactly as the list endpoint would, marshal it,
and compare against the fingerprint published last cycle. What differs is what
changed.

- One query per cycle, not per viewer — SC-002a holds by construction.
- It catches derived change (a job starting, a percentage moving) as naturally as
  a stored write, because it compares the *shown* view rather than the row.
- It cannot drift from what the list returns, because it calls the same
  projection.

Rows that leave the unfinished set are resolved individually: still there means
it went final (publish the final view), gone means it was dismissed (publish a
removal, FR-013). Both are bounded by how many finished this cycle.

**Alternative considered**: announce at each mutation site. Rejected above —
incomplete by construction, since two of the four sources are never written.

**Alternative considered**: publish the changed ids and let each connection
re-project them. Rejected: that is a database read per viewer per change, which
is the multiplication SC-002a forbids.

## 4. The cost when nobody is watching

The diff is skipped entirely when the hub has no subscribers. A server nobody is
looking at does exactly what it did before this feature — no extra query, no
extra projection, no memory held. Worth stating because the reconciler runs every
three seconds for the life of the process.

## 5. Two views of the same change

`submittedBy` is admin-only. A change is therefore rendered **twice** — once as
an admin sees it, once as its owner does — and each subscriber is handed the one
it may have. Twice per change, not once per viewer.

Ownership itself is the rule the list already applies (`ytdlVisibleTo`): your
own, or everything if you are an admin, and an unowned record to admins only.

## 6. Ownership on a connection that outlives its session

A poll re-authenticates on every request. A stream authenticates once and may
then live for hours, so a revoked session or a withdrawn admin flag would keep
working until the reader closed the tab.

**Decision**: re-resolve the session on every heartbeat — once per 15 seconds per
connection, negligible — and end the stream if the session is gone or the user's
admin flag has changed. The client's existing 401 handling then takes over.

## 7. What the client does with a delta

FR-004a: merge what you hold, ignore what you do not. But a genuinely NEW
top-level download must still appear, or an admin watching would never see
somebody else's submission once polling stops.

**Decision**: the event separates `created` from `changed`. A `created` row with
no parent is inserted at the top of the list — the list is ordered newest-first,
so the top is where a new row belongs. Everything else merges into what is
already held and is otherwise ignored. An item (one with a `parentId`) only ever
reaches the open group sheet.

**The connect gap**: a change between "fetch the list" and "the stream is
established" would be lost by both. The server emits a `ready` event once the
stream is up; the client fetches on `ready` rather than before connecting, so the
fetch and the stream overlap instead of leaving a hole. This is also what makes
FR-004b work with no replay: recovery is just re-fetching what you are showing.
