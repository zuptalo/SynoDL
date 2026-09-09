# Contract: `GET /v1/ytdl/stream`

Live updates for YouTube downloads. Stateful mode only.

## Request

```
GET /v1/ytdl/stream
Accept: text/event-stream
X-SynoDL-Session: <token>
```

The session travels in a **header**, never in the query string (Principle III).
This is why the client uses `fetch` + a stream reader rather than `EventSource`.

## Responses

| Status | When | What the client does |
|---|---|---|
| `200` | established | reads frames |
| `401 {"error":"session"}` | no or expired session | returns to login |
| `503 {"error":"stream_limit"}` + `Retry-After: 5` | at the connection cap | polls instead (FR-011) |
| `503 {"error":"unavailable"}` | no orchestrator configured | polls instead (FR-008) |

## Frames

**Ready** — sent once, immediately after the headers.

```
event: ready
data: {}
```

The client fetches the list on this rather than before connecting, so a change
that lands between the fetch and the connection is carried by one or the other
instead of falling between them.

**Update** — one frame per reconcile cycle in which something a reader may see
changed. A cycle in which nothing changed sends nothing.

```
data: {"created":[<download>],"changed":[<download>],"removed":["<requestId>"]}
```

- Each `<download>` is exactly the object `GET /v1/ytdl` returns for that row,
  including `parentId`, `progress` and a group's `counts`.
- All three keys are optional; at least one is non-empty.
- Size is proportional to what changed, never to how much exists (FR-004).

**Heartbeat** — every 15 seconds of quiet.

```
:
```

An SSE comment. It keeps the operator's reverse proxy from closing an idle read
(FR-007), and it is when the server re-checks that the reader may still see what
it is being sent.

**Terminal error** — the session went away mid-stream.

```
event: error
data: {"error":"session_expired"}
```

## Client merge rules

- A row already held is **replaced**.
- A `created` row with no `parentId` is **inserted at the top** — the list is
  ordered newest-first.
- Anything else not already held is **ignored**: history is unbounded and the
  list is paged, so an update is not an instruction to load a page (FR-004a).
- A `removed` id is dropped wherever it is held (FR-013).
- Recovery after a drop is re-fetching what is on screen; the stream replays
  nothing (FR-004b).
