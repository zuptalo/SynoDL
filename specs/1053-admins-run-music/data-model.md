# Data Model: Admins run the music library repair from Settings

## `music_repairs` (SQLite, migration 0040)

The request and its finished outcome (constitution Principle III). Created `IF NOT EXISTS`
like every migration (the drift repair replays them).

| Column | Type | Meaning |
|---|---|---|
| `id` | TEXT PK | 12 hex chars; also the Job's suffix (`music-repair-<id>`) |
| `kind` | TEXT | `check` \| `apply` \| `undo` |
| `plan_id` | TEXT | for `apply`/`undo`: the plan acted on (from the request). For `check`: filled from the worker's `result` event |
| `state` | TEXT | `running` \| `finished` \| `refused` \| `unfinished` |
| `user_id` | INTEGER | `REFERENCES users(id) ON DELETE SET NULL` |
| `user_name` | TEXT | display name at the time, so history survives the user being deleted |
| `started_at`, `finished_at` | INTEGER | unix seconds; `finished_at` NULL while running |
| `summary` | TEXT | JSON of the fixed `Summary`; ≤ 16 KiB; '' when none |

`CREATE UNIQUE INDEX idx_music_repairs_one_running ON music_repairs (state) WHERE state = 'running'`
— inserting a `running` row is acquiring the single slot. Retention: after each insert,
rows beyond the newest 50 are deleted (never the `running` one).

- `finished` — the Job completed and its report was read (or was absent: `summary` empty).
- `refused` — the tool ran and declined (`ok:false`: locked, no such plan, not enough space, plan rejected).
- `unfinished` — the Job failed, hit its deadline, vanished, or could not be created.

## `Summary` (the only view of a run the server holds)

```
Summary {
  kind          "check" | "apply" | "undo"
  ok            bool
  reason        "" | "locked" | "no_plan" | "no_space" | "rejected" | "failed_steps"   (fixed enum)
  planId        string  (matches the plan-id pattern, else dropped)
  check   { tracks, songs, duplicates, moves, retags, covers, playlists, conflicts, nameClashes,
            matched, noMatch, notLookedUp, toSingles, albumKnown, orphanNfo,
            bytesReclaimed, bytesNeeded, freeBytes,
            leftAlone { total, byReason[≤12]{reason,count}, examples[≤20]{path,reason} } }
  apply   { done, skipped, failed, alreadyDone, skippedByReason[≤12], failedExamples[≤10]{path,note} }
  undo    { restored, skipped, skippedExamples[≤10]{path,note} }
}
```
Every integer is clamped to `[0, 10¹²]`; every string is control-character-stripped and
length-capped (paths 200, reasons/notes 120). Exactly one of `check`/`apply`/`undo` is
present, matching `kind`.

## `Progress` (in memory, never stored)

`{ phase: "scan"|"lookup"|"plan"|"apply"|"restore", done, total }` — the latest `progress`
event of a running run; cached for the poll interval and lost on restart without consequence.

## Derived view returned to the client

```
Snapshot {
  available, reason
  current   { id, kind, state, startedAt, startedBy, progress? } | null
  plan      { id, checkedAt, expiresAt, summary, status, canApply, canContinue, canUndo } | null
  history[] { id, kind, state, startedAt, finishedAt, startedBy, planId, headline }
}
plan.status: "ready" | "expired" | "applying" | "applied" | "apply_unfinished" | "undone"
```
See `contracts/api.md`.
