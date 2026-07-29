# Contracts: Statistics API, Task-Create Category, Notification Payload

All endpoints are stateful-mode only, require an authenticated session
(`X-SynoDL-Session`), and follow the existing `request<T>` conventions in
`src/services/api.ts`. Role gating is server-enforced: non-admins are silently
scoped to their own `user_id`; only admins/owner may target another user or all.

## 1. `GET /v1/stats/summary`

Per-user, per-source category counts and average sizes.

**Query params**: none required. Admins/owner receive every user; a non-admin
receives only their own row regardless of params.

**200 response** — each category carries RAW aggregates so the client can combine
categories, sources, and users exactly (an average of averages would be wrong):
```json
{
  "users": [
    {
      "userId": 2,
      "username": "alice",
      "bySource": {
        "catalog": {
          "movie":  { "count": 12, "completed": 12, "sumBytes": 57982058496 },
          "series": { "count": 40, "completed": 38, "sumBytes": 57123064832 },
          "anime":  { "count": 5,  "completed": 5,  "sumBytes": 3670016000 },
          "musicVideo": { "count": 0, "completed": 0, "sumBytes": 0 },
          "music":  { "count": 0, "completed": 0, "sumBytes": 0 },
          "other":  { "count": 0, "completed": 0, "sumBytes": 0 }
        },
        "direct": {
          "movie":  { "count": 3, "completed": 0, "sumBytes": 0 },
          "series": { "count": 0, "completed": 0, "sumBytes": 0 },
          "anime":  { "count": 0, "completed": 0, "sumBytes": 0 },
          "musicVideo": { "count": 2, "completed": 2, "sumBytes": 209715200 },
          "music":  { "count": 8, "completed": 8, "sumBytes": 75497472 },
          "other":  { "count": 1, "completed": 0, "sumBytes": 0 }
        }
      }
    }
  ]
}
```

Rules:
- `count` = all rows for that (user, source, category), incl. paused/canceled.
- `completed` = rows with a known size; `sumBytes` = sum of those sizes.
- The client derives the average as `sumBytes / completed`, showing "—" when
  `completed == 0` (FR-015 — never 0).
- The combined **all-sources** view and per-source/overall averages are derived
  **client-side** by summing the raw fields across the two source objects (and,
  for an admin's "all users", across users) — exact, because raw counts add up.
- Every visible user is present with both sources zero-filled across all six
  categories, so the client renders a stable grid.

## 2. `GET /v1/stats/timeseries`

Daily download counts for building the historical graph. The client re-buckets
days into week/month/year/all-time locally (no refetch on granularity switch).

**Query params**:
| param | values | default | notes |
|---|---|---|---|
| `source` | `catalog` \| `direct` \| `all` | `all` | filters rows by source |
| `userId` | a user id \| `all` | caller | **admins/owner only**; ignored (forced to self) for non-admins |

**200 response**:
```json
{ "userId": "all", "source": "all",
  "days": [ { "date": "2026-07-01", "count": 3 }, { "date": "2026-07-02", "count": 0 } ] }
```

Rules:
- One entry per day from the first recorded download to today; empty days are
  present as `count: 0` so the graph shows gaps (FR-020 / US3 scenario 4).
- `date` is the UTC calendar day of `created_at`. Coarser buckets (week/month/
  year) and "all-time" are summed client-side in the viewer's local time.

## 3. Errors (both endpoints)

| Status | Body | When |
|---|---|---|
| 401 | `{"error":"session"}` | missing/expired session (existing 401→login flow) |
| 403 | `{"error":"forbidden"}` | stateless/legacy mode where stats don't apply |
| 200 | empty `users`/`days` | authenticated but no history yet (fresh start) — not an error |

## 4. Task-create category (extends existing `POST /v1/tasks`)

The new-task modal adds an optional media category so a direct download can be
categorized (with a heuristic fallback server-side).

**JSON body (URI add)** — adds one field:
```json
{ "uris": ["magnet:?xt=..."], "destination": "movies/Some Movie 2024",
  "category": "movie" }
```
**Multipart (torrent) add** — adds one form field: `category`.

Rules:
- `category` is optional; allowed values are the six enum strings or `auto`
  (or absent) meaning "use the server heuristic".
- The server records the resulting `download_history.source = "direct"` row with
  the chosen category, else `mediaclass.Classify(destination, fileName)`, else
  `other`. **`category` is ignored for catalog sends** (`/v1/source/send`), which
  always use the catalog `media_type`.
- Out-of-range values are treated as `auto` (never rejected — a bad category must
  not fail a download).

## 5. Notification payload (extends the Web Push body built in `watcher.go`)

The service-worker payload shape is unchanged (`{title, body, taskId}`); only how
`body` is composed changes. Built **per subscriber**:

| Recipient | `body` |
|---|---|
| Owner of the download (any role) | readable title, e.g. `X-Men '97 · S01E01` |
| All-scope admin/owner, download owned by someone else | readable title + ` · added by <username>` |
| Non-admin, not the owner | *not sent* (existing scope gate already filters) |

- `title` (the notification heading) stays the fixed event string
  (`Download added` / `Download complete` / `Download failed`).
- The readable title comes from `tasktitle.Title(t.Name, t.Destination, t.URI)`;
  when no title is derivable it falls back to the raw name (never empty) (edge
  case in spec).
- Payload MUST contain no URIs, sids, or credentials (Principle III); `<username>`
  is the only added identity and only for all-scope admin/owner recipients.
