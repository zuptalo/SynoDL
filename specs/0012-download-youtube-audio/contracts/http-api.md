# Contract: HTTP API

**Spec**: [spec.md](../spec.md) | **Date**: 2026-09-06

Three endpoints, all stateful-mode only and behind the existing session gate.
They follow the house style: `mux.Handle("VERB /v1/...", handler(d))` in
`router.go`, one handler file with a sibling `_test.go`, dependencies behind a
narrow interface so tests pass fakes.

## POST /v1/ytdl

Submit a download.

```jsonc
// request
{
  "url":  "https://youtu.be/<id>",   // required
  "mode": "music"                    // required: "music" | "music-video"
}
```

```jsonc
// 202 Accepted
{
  "requestId": "…",
  "scope":     "single",     // derived server-side: single | playlist | channel
  "mode":      "music",
  "state":     "scheduled"
}
```

**Behaviour**

- The URL is validated against the host allowlist BEFORE anything else. A host
  outside it is `400` with a plain-language message and no Job is created
  (FR-003).
- Scope is **derived from the URL, never supplied by the client** — a channel URL
  is normalised to its videos tab (FR-014). Accepting a client-supplied scope
  would let a caller aim a channel run at a playlist URL and vice versa.
- A duplicate in-flight request (same normalised URL + mode, still `scheduled` or
  `started`) is `409` with the existing `requestId` (FR-023).
- Over the concurrency limit the Job is still created; the cluster queues it and
  it stays `scheduled`. The server does not maintain a queue of its own.
- `503` when the server is not running in a cluster, with a message saying the
  feature is unavailable in this deployment — not a `500`.

**Errors**: `400` bad/blocked URL or unknown mode · `401` no session · `409`
duplicate in flight · `503` no orchestrator.

## GET /v1/ytdl

List current downloads and unresolved failures — the feed the Tasks list merges.

```jsonc
// 200 OK
{
  "downloads": [
    {
      "requestId":   "…",
      "url":         "https://youtu.be/<id>",
      "mode":        "music",
      "scope":       "single",
      "state":       "started",       // scheduled | started | completed | failed
      "submittedBy": "kamran",        // admins only, matching the Tasks rule
      "submittedAt": 1757000000,
      "reason":      ""               // populated only when state is "failed"
    }
  ],
  "degraded": false                   // true when the orchestrator is unreachable
}
```

**Behaviour**

- Live rows come from ONE label-selector LIST against the orchestrator
  (`managed-by=synodl,kind=ytdl`), merged with unresolved failure rows from the
  store. Nothing is fetched per request id (FR-019, and it keeps the cost flat
  regardless of history size).
- A Job observed in a terminal `failed` state is recorded to the store,
  idempotently by `requestId` — state is polled, not evented, so the same failure
  will be seen more than once.
- **No progress fields exist in this shape at all** (FR-017). They are absent
  from the contract rather than present-and-zero, so nothing can quietly start
  populating them later.
- If the orchestrator is unreachable, stored failures are still returned and
  `degraded` is `true`; the endpoint does not fail wholesale.

## DELETE /v1/ytdl/{requestId}

Dismiss a finished download: deletes the stored failure record and the
already-terminal Job.

- `204` on success · `404` if unknown · `409` if the download is still in flight
  (dismissing a running download would strand its worker — cancellation is
  deliberately out of scope for this spec).
- **Never touches the media library.** Dismissing removes the record of a
  download, never a downloaded file.

## Client-visible shape change

`GET /v1/tasks` is **unchanged**. The client merges the two feeds rather than the
server merging them, which keeps the NAS task path — the app's most load-bearing
endpoint — untouched by this feature.
