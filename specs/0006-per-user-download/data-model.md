# Phase 1 Data Model: Per-User Download Statistics

## New table: `download_history` (migration 0013)

Append-only event log of downloads initiated through SynoDL. One row **per file**
(a multi-episode catalog send inserts one row per episode). Written at create
time; size backfilled on completion. This is the single source of truth for all
statistics. It is NOT a live task mirror — the NAS remains authoritative for
current task state (Principle III).

```sql
-- 0013 — durable per-download history for the Statistics section (spec 0006).
-- One row per downloaded file, written at create time (so paused/canceled still
-- count, matching the daily-limit accounting) and backfilled with the real size
-- by the completion watcher. Fresh start: only rows created after rollout exist.
CREATE TABLE download_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source       TEXT    NOT NULL,               -- 'catalog' | 'direct'
    category     TEXT    NOT NULL,               -- movie|series|anime|music_video|music|other
    destination  TEXT    NOT NULL DEFAULT '',    -- folder; correlation key for size backfill
    task_name    TEXT    NOT NULL DEFAULT '',    -- expected file name; correlation key
    created_at   INTEGER NOT NULL,               -- unix secs, when the download was added
    completed_at INTEGER,                         -- unix secs, set when the watcher sees it finish
    size_bytes   INTEGER,                         -- real size; NULL until completed
    task_id      TEXT                             -- DSM task id once correlated (diagnostic)
);
CREATE INDEX idx_download_history_user       ON download_history(user_id, created_at);
CREATE INDEX idx_download_history_correlate  ON download_history(destination, task_name, completed_at);
```

### Field semantics & validation

| Field | Rule |
|---|---|
| `source` | exactly `catalog` or `direct`; set by the writing handler, never client-supplied for catalog. |
| `category` | one of the six enum values. Catalog: from `source_downloads.media_type` (`movie`/`series`/`anime`). Direct: client-chosen override, else `mediaclass.Classify(...)`, else `other`. |
| `created_at` | server clock at create; drives counts and the timeseries. |
| `size_bytes` | NULL until completion; set to the finished task's `Size`. Only non-NULL rows enter size averages. |
| `completed_at` | NULL until the watcher observes `finished`; also marks a row as size-resolved so it isn't matched twice. |
| `user_id` | always set (we only record attributable downloads). `ON DELETE CASCADE` → history removed with the user (FR-012). |

### Lifecycle / state transitions

```
create (handler)                 completion (watcher)
─────────────────                ────────────────────
INSERT row                       match oldest row WHERE destination=? AND task_name=?
  size_bytes = NULL      ─────▶    AND completed_at IS NULL, ORDER BY created_at LIMIT 1
  completed_at = NULL              UPDATE size_bytes=?, completed_at=?, task_id=?
```

- A row never leaves the table except via user-delete cascade (append-only).
- Paused/canceled/failed: the row stays with `size_bytes = NULL` forever → counts
  but no size sample.
- No completion match (fast finish before first poll, or name mismatch): row stays
  size-less; internally logged as a no-match, never an error.

## Repository functions (`server/internal/store/history_repos.go`)

```go
// Write path
func (s *Store) AddDownloadHistory(rec DownloadHistory) error            // INSERT at create time
func (s *Store) CompleteDownloadHistory(dest, name string, size, now int64) (bool, error)
                                                                         // backfill oldest match; false if none

// Read/aggregate path
func (s *Store) StatsSummary(userIDs []int64) ([]UserSourceStats, error) // GROUP BY user_id, source, category
func (s *Store) StatsDaily(userIDs []int64, source string) ([]DayCount, error) // GROUP BY date(created_at)
```

```go
type DownloadHistory struct {
    UserID      int64
    Source      string // "catalog" | "direct"
    Category    string
    Destination string
    TaskName    string
    CreatedAt   int64
}

type UserSourceStats struct {
    UserID   int64
    Username string
    Source   string            // "catalog" | "direct"
    Counts   map[string]int    // category -> count (all rows)
    AvgSize  map[string]int64  // category -> avg size in bytes (completed rows only); overall under "" or "overall"
}

type DayCount struct {
    Date  string // "YYYY-MM-DD" (UTC of created_at)
    Count int
}
```

Notes:
- `AVG` computed only over `size_bytes IS NOT NULL`; a category with no completed
  rows yields no entry → the API returns it as not-available (FR-015).
- Counts include every row (paused/canceled included) — `COUNT(*)`, not gated on
  size (FR-023).
- The `all`/combined source view is derived by summing `catalog` + `direct`
  server-side (or client-side); the summary returns both source rows so the client
  can show per-source and combined.

## Touched existing tables (no schema change)

- `users` — read for role (`is_admin`), owner = `MIN(id)`, and username join. User
  deletion already cascades (`sessions`, `download_events`); `download_history`
  adds the same cascade.
- `source_downloads` — read by the watcher/handler to resolve catalog
  `owner_user_id` + `media_type` by destination (already exists).
- `download_events` — unchanged; remains the daily-limit ledger (catalog-only).
  `download_history` counts are designed to agree with it for catalog rows.
- `task_claims` — unchanged; still the name-based notification-attribution bridge.

## New pure helpers (unit-tested, no DB)

- `server/internal/tasktitle/tasktitle.go` — `Title(name, destination, uri string)
  (title, episode string)`. Go port of `src/services/task-title.ts` (same
  folder-title derivation + `SxxEyy` extraction; keep the same test cases,
  including the underscore/`SE_RE` boundary behavior).
- `server/internal/mediaclass/mediaclass.go` — `Classify(destination, fileName
  string) string`. Folder-parent map (`movies`→movie, `tv`/`series`→series,
  `anime`→anime, `music`→music, `music-video`/`mv`→music_video) with an
  audio-extension fallback (`.mp3/.flac/.m4a/…`→music); default `other`.

## Client types (`src/services/api.ts`)

```ts
export interface StatCategoryStat { count: number; completed: number; sumBytes: number }
export interface StatUserSummary {
  userId: number; username: string;
  bySource: { catalog: Record<Category, StatCategoryStat>;
              direct:  Record<Category, StatCategoryStat> };
}
export interface StatsTimeseries { userId: number | 'all'; source: string; days: { date: string; count: number }[] }
export function avgSize(s): number | null // sumBytes/completed, or null when completed==0
```
`Category = 'movie'|'series'|'anime'|'musicVideo'|'music'|'other'`. Averages, the
combined-source ("all") view, cross-user aggregation, and coarser time buckets are
all computed client-side from the raw fields (`avgSize`, `stats-buckets.ts`).
