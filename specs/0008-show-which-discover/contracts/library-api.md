# Contract: ownership on catalog results, and season/episode detail

Phase 1 for [plan.md](./plan.md). Two surfaces. Both are authenticated as the signed-in
SynoDL user and act through the operator's single stored NAS connection.

## 1. Ownership on catalog results *(amended)*

`POST /v1/source/search` and `GET /v1/source/title/{id}` decorate each item in place, as
`sourceId`/`sourceName` already are. Drivers never set these fields.

```jsonc
{
  "id": "1:4821",
  "title": "Attack on Titan 2013",
  "ownership": "owned"        // unknown | absent | owned | downloading
}
```

**Breaking change from 0.3.0.** The boolean `inLibrary` is replaced by `ownership`. A
boolean cannot express the two states added by FR-001b and FR-010c — "a download is running"
and "not checked yet" — and both must be distinguishable from "you have it".

| Rule | Requirement |
|---|---|
| Omitted or `unknown` → render **no** marker | FR-010c |
| `downloading` outranks `owned` | FR-019a |
| A failed NAS read yields `unknown`, never `absent` | FR-009 |
| Only titles in the current response are verified | FR-010b |

## 2. `GET /v1/library/title`

Season and episode detail for one catalog title, for the download-options sheet.

**Query**: `type=movie|series|anime`, `title=<raw catalog title>`

The title is matched by the same rules as the grid; a client **cannot** name a path
(FR-025a). Lookups are rate-limited per user (FR-025b).

```jsonc
{
  "ownership": "owned",
  "seasons": [
    { "season": 1, "episodes": [1,2,3,4,5,6], "videoFiles": 6 },
    { "season": 2, "episodes": [1,2],         "videoFiles": 2 },
    { "season": 0, "episodes": [],            "videoFiles": 1 }
  ]
}
```

| Field | Meaning |
|---|---|
| `seasons` | Only seasons actually holding video. Absent seasons are simply not listed |
| `episodes` | Numbers read from the file names, sorted and de-duplicated |
| `videoFiles` | Count including files whose numbering could not be read |

**No `total` and no `complete`, by contract** (FR-016a). The catalog's episode count is not
reliable, so the response never implies one. `episodes: []` with `videoFiles > 0` is valid
and means "present, numbering unreadable" (FR-016b) — a client MUST render that as present.

A movie returns `ownership` with `seasons: []` (FR-014). A folder that cannot be read
returns `ownership: "unknown"` and `seasons: []` with **HTTP 200** — the sheet stays fully
usable and shows no markers (FR-017), so this is not an error condition.

### Status codes

| Code | When |
|---|---|
| 200 | Answered, including the degraded "unknown" case |
| 400 | Missing or unrecognised `type`, or an empty `title` |
| 401 | No SynoDL session |
| 429 | Per-user lookup budget exceeded (FR-025b) |

Note there is **no 404**: "not present" is a successful answer, not a missing resource.

## Non-goals

- No endpoint accepts or returns a NAS path chosen by the client.
- No endpoint lists a folder's contents verbatim; only derived presence is exposed.
- Nothing here is persisted, and nothing is cached client-side (Principle IV).
