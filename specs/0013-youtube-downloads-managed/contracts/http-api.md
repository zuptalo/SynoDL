# HTTP contract: spec 0013

Typed `/v1` endpoints only — never a transparent passthrough. Every endpoint
below requires a SynoDL session; ownership is enforced per FR-007/FR-008 with
the admin exception stated positively in FR-009a.

## Ownership rule, applied uniformly

| Caller | List | Detail / items | Retry | Dismiss |
|---|---|---|---|---|
| Owner | own rows | yes | yes | yes |
| Admin | all rows, attributed | yes | yes | yes |
| Other user | not listed | 404 | 404 | 404 |

**404, not 403** for another user's download: a 403 confirms the row exists,
which FR-008 forbids. The same applies to a `requestId` that never existed, so
the two are indistinguishable from outside.

---

## `POST /v1/ytdl` — submit (existing, behaviour changed)

Request unchanged: `{ "url": string, "mode": "music" | "music-video" }`.

Changed behaviour:
- Creates a durable record before anything else (FR-001).
- Does **not** create a Job. The record enters `queued` (or `resolving` for a
  playlist or channel) and the reconciler admits it (FR-021, FR-022).
- Duplicate in-flight for the same `(video_id, mode)` is still refused `409`
  (0012 FR-023, unchanged).
- The status stays **`202 Accepted`**, as the shipped handler already returns.
  Nothing in this spec asks for it to change, and the client checks it.

```json
202 { "requestId": "…", "kind": "single"|"group", "scope": "…", "mode": "…", "state": "queued"|"resolving" }
409 { "error": "that link is already downloading", "requestId": "…" }
400 { "error": "that link is not a supported YouTube address" }
503 { "error": "downloading from YouTube is not set up on this server" }
```

## `GET /v1/ytdl` — list (existing, behaviour changed)

Now ownership-filtered (FR-007) and paged (FR-006a). Group items are **not**
returned (FR-019b) — a group appears as one row.

```
GET /v1/ytdl?cursor=<opaque>&limit=<n>
```

```json
200 {
  "downloads": [ DownloadView ],
  "nextCursor": "…"|null,
  "degraded": false
}
```

`degraded` keeps its 0012 meaning: the live half could not be read, stored
records are still returned. Distinct from a download having failed (FR-032b).

## `GET /v1/ytdl/{requestId}` — detail (new)

Every fact for one download (FR-030).

```json
200 {
  "requestId": "…", "kind": "single",
  "url": "…", "title": "…", "uploader": "…", "artwork": "…",
  "mode": "music", "scope": "single", "state": "downloading",
  "progress": 0.42,
  "hasLyrics": true, "lyricsLang": "en",
  "groupName": null, "origin": "direct",
  "reason": null, "attempts": 1,
  "submittedBy": "anna",
  "createdAt": 1764700327, "finishedAt": null
}
```

- `progress` present only while `downloading`; omitted otherwise (FR-013).
- `finishedAt` omitted, never `0`, until final (FR-033).
- `submittedBy` present for admins only (FR-009).
- `reason` is plain language, always (FR-032).

## `GET /v1/ytdl/{requestId}/items` — a group's items (new)

Paged, same `DownloadView` shape, plus the group's aggregate (FR-019, FR-019a).

```json
200 {
  "group": { "requestId": "…", "title": "Lo-fi Beats", "scope": "channel",
             "state": "downloading",
             "counts": { "total": 340, "completed": 38, "failed": 2, "remaining": 300 } },
  "items": [ DownloadView ],
  "nextCursor": "…"|null
}
```

`404` if `requestId` is not a group the caller may see.

## `POST /v1/ytdl/{requestId}/retry` — retry (new)

Only for `failed` (FR-028). Moves the row back to `queued`, increments
`attempts`, clears `reason` and `finishedAt`. Stays one row (FR-029).

```json
202 { "requestId": "…", "state": "queued", "attempts": 2 }
409 { "error": "that download has not failed" }
404 (not yours, or no such download)
```

Retrying a **group** re-queues only its failed items, not the whole group.

## `DELETE /v1/ytdl/{requestId}` — dismiss (existing, behaviour changed)

- No longer refuses while running (FR-005c; supersedes 0012's behaviour).
- On a group, removes the group and every item record (FR-005a).
- Queued rows leave the queue and never start; running workers are left to
  finish and their records go when they do (FR-005b).
- Never touches saved files (FR-005).

```json
204 (no content)
404 (not yours, or no such download)
```

## `GET /v1/ytdl/thumb` — artwork proxy (existing, behaviour changed)

Now requires a session (FR-009d). The host allowlist is unchanged — it is this
feature's own list and knows nothing about the download sources.

```
401 when unauthenticated (previously served to anyone)
```

---

## Worker contract

Not HTTP, but the other interface this feature defines.

**Download worker** — argv gains, over spec 0012's verified recipe:

| Addition | Requirement |
|---|---|
| `--newline` + `--progress-template` with a fixed sentinel prefix | R2, FR-010 |
| Album metadata carrying the sanitised group name, as a discrete argument | FR-036, FR-037, FR-038 |

Unchanged and load-bearing: the URL is the final argv element after `--`, it
appears exactly once, and nothing concatenates it into another string.

**Expansion worker** — a new, separate short-lived Job:

| Property | Requirement |
|---|---|
| Lists entries without resolving each one | R4 |
| Prints one line per entry in a template SynoDL defines | R4 |
| Mounts **no** media library | It writes nothing; a worker mounts one library only when it needs one |
| Bounded by its own deadline | FR-016 |

**Reading worker output** — bounded in size (FR-013e) and in frequency, driven
by the reconciler rather than by request handlers (FR-013f). Lines without the
sentinel prefix are ignored, never guessed at.

## Orchestrator permission delta

| Resource | Verbs | Status |
|---|---|---|
| `batch/jobs` | create, get, list, delete | unchanged (0012) |
| `pods` | get, list | unchanged (0012) |
| `pods/log` | **get** | **added by this spec** |

Still a namespaced Role, never a ClusterRole. Still no `secrets`, no
`pods/exec`, no `pods/attach`, no direct pod create/delete.
