# Contract: `GET /v1/tasks/stream` (Server-Sent Events)

New endpoint. Streams task-list + global-stats snapshots so the client reflects NAS state live.

## Request

```
GET /v1/tasks/stream
X-Syno-Sid: <session id>        # REQUIRED — same header as every /v1 call. NEVER in the URL/query.
Accept: text/event-stream
```

- No query parameters. No request body.
- Auth: identical to other `/v1` endpoints — missing/invalid sid → session-expiry handling.

## Response (success)

```
200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Body is an SSE stream, flushed after every write:

- **Snapshot event** (every ~1s while data is available; the server MAY skip re-sending an identical
  consecutive snapshot):
  ```
  data: {"tasks":[...],"stats":{"downloadSpeed":0,"uploadSpeed":0}}\n\n
  ```
  The JSON is byte-for-byte the same shape as the `GET /v1/tasks` body (see tasks-list.md), including
  the new `errorDetail` field.

- **Heartbeat** (at least every ~15s when no snapshot was sent): an SSE comment, ignored by clients:
  ```
  :\n\n
  ```

- **Terminal auth error** (session expired mid-stream): a final event, then the connection closes:
  ```
  event: error
  data: {"error":"session_expired"}\n\n
  ```

The stream stays open until the client disconnects, the server is shutting down, or a terminal error
occurs.

## Response (error, before the stream opens)

| Status | When | Body |
|---|---|---|
| `401 Unauthorized` | missing/invalid/expired sid at connect time | typed error JSON (existing shape) |
| `503 Service Unavailable` + `Retry-After` | concurrent-stream cap reached | typed error JSON; client falls back to polling and retries with backoff |
| `405 Method Not Allowed` | non-GET | — |

## Server behavior (normative)

- Poll cadence to the NAS is a **fixed server constant (~1s)** — never client-tunable.
- Reuses ONLY the existing allowlisted `syno.Client.ListTasks`; adds no DSM API.
- Holds the `sid` only for the connection; persists nothing; logs route + outcome + lifecycle
  (open/close/heartbeat/shed) — never the sid, credentials, OTP, or full task URIs.
- Stops promptly on client disconnect (request context cancellation) and on server shutdown.
- Concurrent streams are globally bounded; excess connections are shed with `503`.

## Client behavior (normative)

- Consume via `fetch()` + `ReadableStream` reader (so the sid is a header) — not `EventSource`.
- Open only while the Tasks view is mounted AND `document.visibilityState === 'visible'`; abort on
  hide/unmount.
- On connect failure, mid-stream error/close (non-auth), or `503`: fall back to the existing 3s
  polling immediately and retry the stream with capped exponential backoff.
- On `401`/terminal `session_expired`: do not fall back — trigger the existing session-expiry →
  return-to-login flow.
- Never hold more than one stream at a time.
