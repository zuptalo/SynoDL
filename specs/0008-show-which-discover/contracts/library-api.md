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

## 2. Season and episode detail on `GET /v1/source/title/{id}`

Season detail rides on the existing title endpoint rather than a lookup of its own.

**This is a security decision, not just a design one.** That endpoint already
resolves the title through the caller's own source access, so a user can only ask
about a title they could already open. A free-standing lookup keyed by title text
would have been exactly the way around a user's catalog narrowing that FR-025c
forbids — and it would have needed its own rate limit and its own path-injection
guard to claw back what this design gets for free (FR-025a, FR-025b, FR-025c).

```jsonc
{
  "id": "1:4821",
  "title": "Friends 1994 - 2004",
  "qualities": [ /* unchanged */ ],
  "ownership": "owned",
  "seasons": [
    { "season": 1, "episodes": [1,2,6], "videoFiles": 3 },
    { "season": 2, "episodes": [1],     "videoFiles": 1 }
  ]
}
```

| Field | Meaning |
|---|---|
| `seasons` | Only seasons actually holding video. Absent seasons are simply not listed |
| `episodes` | Numbers read from the file names, sorted and de-duplicated |
| `videoFiles` | Count including files whose numbering could not be read |

**No `total` and no `complete`, by contract** (FR-016a). The catalog's episode
count is not reliable, so the response never implies one. `episodes: []` with
`videoFiles > 0` is valid and means "present, numbering unreadable" (FR-016b) — a
client MUST render that as present.

A movie omits `seasons` entirely (FR-018). A folder that cannot be read returns
`ownership: "unknown"` with no seasons and **HTTP 200** — the download options stay
fully usable and show no markers (FR-017), so this is not an error condition.

A title still downloading returns `ownership: "downloading"` and **no seasons**:
detail read from a half-written folder would be taken for what the user has.

## Non-goals

- No endpoint accepts or returns a NAS path chosen by the client.
- No endpoint lists a folder's contents verbatim; only derived presence is exposed.
- Nothing here is persisted, and nothing is cached client-side (Principle IV).
