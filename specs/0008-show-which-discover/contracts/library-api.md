# Contract — Library awareness

**Feature**: 0008 — Show which Discover titles you already have

Two changes to existing contracts and one new endpoint. All are **stateful-mode only** — they
are registered inside the `d.Stateful` branch of `server/internal/api/router.go`, alongside the
rest of `/v1/source/*`, and do not exist in legacy stateless mode.

---

## 1. `POST /v1/source/search` — amended response

Each item in `items[]` may now carry one additional field.

```jsonc
{
  "page": 1,
  "pages": 12,
  "items": [
    {
      "id": "1:88213",
      "type": "movie",
      "title": "Dune 2021",
      "posterUrl": "...",
      "imdbScore": 8.0,
      "inLibrary": true          // NEW — omitted when false
    }
  ],
  "degraded": []
}
```

| Field | Type | Meaning |
|---|---|---|
| `inLibrary` | `boolean` (optional) | A folder for this title already exists under the configured parent. **Absent means "not present, or not known"** — the client MUST treat absence and `false` identically. |

Guarantees:

- The field is set by the API layer from the library snapshot, never by a source driver.
- When the snapshot could not be built (NAS unreachable, parent missing or unreadable), **every**
  item omits the field and the request otherwise succeeds unchanged (FR-009). A failed scan is
  never an error response and never a partial failure the client must handle.
- Setting the field adds no NAS round-trip to a request served from a warm snapshot (SC-009).
- Existing clients ignore an unknown field, so this is backward compatible.

---

## 2. `GET /v1/library/title` — new

Answers "do I have this, and which seasons?" for one title the user has opened.

**Request**

| Query param | Required | Value |
|---|---|---|
| `type` | yes | `movie` \| `series` \| `anime` — selects the movies or TV parent |
| `title` | yes | The raw catalog title, exactly as it appears in `CatalogTitle.title` (year included). Names a folder *inside* the configured parent; see the rejection rule below. |

```
GET /v1/library/title?type=series&title=Friends%201994%20-%202004
```

**Response `200`**

```jsonc
{
  "inLibrary": true,
  "seasons": [
    { "season": 1, "files": 24 },
    { "season": 2, "files": 0 }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `inLibrary` | `boolean` | A folder for this title exists |
| `seasons` | `array` | Seasons found on disk, ascending. Always present; `[]` for a movie, for a series with no detectable seasons, or when `inLibrary` is `false` |
| `seasons[].season` | `int` | Season number |
| `seasons[].files` | `int` | Episode files counted. **`0` means "not counted", never "empty"** (FR-016) |

**Deliberately absent**: the folder's name or path. The client needs *whether* and *which*, never
*where* (FR-025).

**Status codes**

| Code | When |
|---|---|
| `200` | Always, on any successful auth — including when nothing is found |
| `400` | `type` missing/unrecognised, `title` empty, or `title` would escape the parent (see below) |
| `401` | No valid SynoDL session |
| `409` | No download source configured, so no parents are known |
| `429` | The caller exceeded the per-user lookup rate (FR-025b) |

**Rejecting, not sanitising** (FR-025a): `title` is refused with `400` if it contains a path
separator (`/` or `\`), a parent-directory reference (`..`), a control character, or is `.`/`..`
or empty after trimming. It is deliberately **not** cleaned up and answered — silently rewriting a
hostile input into a different folder and returning an answer for *that* folder is the failure mode
this rule exists to prevent. Note this is a stricter rule than `sanitizeFolderName`, which replaces
those characters because it is building a folder for a title SynoDL itself chose; here the value is
client-controlled, so it is validated rather than repaired.

There is **no `5xx` for a NAS failure.** An unreachable NAS, a missing parent, or a permission
error all return `200` with `{"inLibrary": false, "seasons": []}` (FR-009, FR-017). The caller
cannot distinguish "you don't have it" from "we couldn't look", and does not need to: both mean
"show no markers".

**Authorization**: requires a signed-in user (`d.requireUser`). It does **not** require a folder
grant on the parent — the signal is instance-wide by design (see plan.md, Complexity Tracking).
It grants no new ability: sending is still governed by `destinationAllowed` (FR-027).

**Rate limiting** (FR-025b): bounded per user via the existing `httpx` limiter already applied to
`POST /v1/session`. Every call can cost a NAS listing, so an unbounded endpoint would let one
signed-in client generate arbitrary load against the operator's NAS. Exceeding the bound returns
`429`; the client treats it exactly like a failed lookup — no markers, no error shown (FR-017).

**Content rating** (FR-025c): this route is held to the same line as `GET /v1/source/title/{id}`,
which today resolves any title id for any signed-in user without applying their content rating.
This endpoint therefore neither widens nor narrows that existing behaviour. The pre-existing gap is
recorded in the spec's Out of Scope section rather than silently inherited.

**Logging**: no folder name, file name, or path from the NAS may appear in a log line, error
payload, or metric on this path (FR-026).

---

## 3. `GET` / `PUT /v1/source/prefs` — amended

```jsonc
{
  "preferredQuality": "1080p",
  "hideOwned": false        // NEW
}
```

| Field | Type | Meaning |
|---|---|---|
| `hideOwned` | `boolean` | Hide titles already present from the Discover grid. Defaults to `false`, which is today's behaviour, so existing accounts are unaffected (FR-024). |

`PUT` accepts the field alongside the existing ones and persists it with them in a single upsert.
An omitted field leaves the stored value unchanged, matching how the endpoint already behaves.

---

## Client-side contract note

`hideOwned` filters in `useSourceCatalog.fetchPage()`, at the same point `comingSoon` items are
already dropped. Because the source cannot filter on ownership upstream, a returned page may be
almost entirely owned; the client therefore fetches further pages until the grid is filled or the
catalog is exhausted (FR-023a), with a bound so an exhausted or heavily-owned catalog cannot spin.
