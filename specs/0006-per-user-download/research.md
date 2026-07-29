# Phase 0 Research: Per-User Download Statistics and Richer Notifications

All four product ambiguities were resolved during `/speckit-clarify` (recorded in
spec.md → Clarifications). This document records the *technical* decisions that
follow, each with rationale and rejected alternatives. There are no remaining
`NEEDS CLARIFICATION` items.

## D1 — Where download history is written (counts vs sizes)

**Decision**: Write a `download_history` row at **task-create time** (in
`source_handlers.go` for catalog, `stateful_handlers.go` for direct) with
`size_bytes = NULL`. **Backfill the real size** in the completion watcher
(`watcher.go`) by matching the finished task to its size-less history row.

**Rationale**: Counts must include paused/canceled downloads and must agree with
the daily-limit accounting, which is already recorded at send time
(`AddDownloadEvents`). Writing history at create time makes counts authoritative
and consistent with the limit. The real, final file size is only known when the
NAS reports a finished task — the watcher is the single place that already
observes this, so it backfills. This mirrors the existing division of labor:
`download_events` (create-time counts) + watcher (completion notifications).

**Alternatives rejected**:
- *Insert history only in the watcher on first sight*: a download canceled before
  the next poll would be counted by the daily limit but missing from history —
  counts would disagree with the limit. Rejected.
- *Use the resolved size known at catalog send time*: available for catalog only,
  and it is the advertised/estimated size, not the real on-disk size; the
  requester chose completed-only real sizes. Rejected (kept as an impossible-for-
  direct, less-accurate path).

## D2 — Correlating a completed task back to its history row

**Decision**: Store `destination` (folder) and `task_name` (expected file name) on
each history row at create time. On completion the watcher runs the equivalent of
`ClaimOwner`: update the oldest size-less row with matching `(destination,
task_name)` — setting `size_bytes`, `completed_at`, and the correlated `task_id`.

**Rationale**: DSM's create call returns no task id, so history cannot be keyed by
task id at insert. `(destination, task_name)` is the same correlation key the
existing attribution already relies on (name-based `task_claims`,
destination-based `source_downloads`). For a multi-episode catalog send, each file
is a distinct row with a distinct expected file name (the last path segment of the
resolved link), so per-episode sizes land on the right rows.

**Alternatives rejected**:
- *Key history by DSM task id*: not available at create time; would force
  watcher-only insertion (see D1). Rejected.
- *Match by destination only*: ambiguous for multi-episode sends (many rows share
  one folder). Rejected in favor of destination + name, with graceful degradation
  (unmatched ⇒ size stays NULL ⇒ excluded from averages).

**Known limitation** (documented, acceptable): if the resolved file name differs
from the DSM task name, or a task finishes before the watcher's first poll, size
backfill is skipped and that download is excluded from size *averages* only — it
still counts. This is logged internally as a no-match, never surfaced as an error.

## D3 — "Start fresh" with no backfill of old data

**Decision**: History begins accumulating at rollout; no migration seeds it from
`download_events` / `source_downloads`.

**Rationale**: The requester chose accuracy over a non-empty day-one graph. Old
`download_events` rows lack size/type/name; `source_downloads` is deduplicated per
folder (re-sends overwritten), so any backfill would undercount and show size-less
records as if complete. Because history is only written by the new create-time
code, "fresh start" is automatic — no special migration logic and no risk of
importing misleading data.

## D4 — Direct-download categorization (heuristic + override)

**Decision**: New pure Go helper `mediaclass.Classify(destination, fileName)`
returns `movie | series | anime | music_video | music | other` from the
destination's parent folder (extending the client's `MEDIA_PARENT` set with music
folders) and the file extension (audio extensions ⇒ music). The new-task modal
adds a category `ion-select` defaulting to **Auto**; when the user picks a
category the create request carries it and it overrides the heuristic. Catalog
downloads ignore the heuristic and use the catalog `media_type`
(movie/series/anime).

**Rationale**: Direct tasks have no catalog metadata; a folder+extension heuristic
covers the common self-organized-library case at zero friction, and an explicit
override guarantees accuracy when it matters (FR-014/FR-015). Keeping it a pure
function makes it unit-testable and mirrors `task-title.ts`'s pure design.

**Alternatives rejected**: mandatory picker on every add (friction on quick adds);
no override (permanent misclassification); no categorization (loses the
music/video breakdown the requester asked for). All rejected per the clarify
answer.

## D5 — Readable, per-subscriber notification body

**Decision**: Port `src/services/task-title.ts` to a pure Go package
`tasktitle` (`Title(name, destination, uri) (title, episode string)`), table-
tested against the same cases as the TS unit tests. In `watcher.poll`, compute the
readable title from the task and pass it as the notification body. In
`notifyEvent`, build the payload **inside the per-subscriber loop**: for an
all-scope (admin/owner) subscriber viewing a download owned by *someone else*,
append the owner's username (resolved once via `GetUserByID`); a subscriber never
sees "added by <themselves>".

**Rationale**: The readable-title logic already exists and is battle-tested on the
client; porting keeps parity (a shared behavior with one canonical set of cases).
Per-subscriber payload construction is required because the same event fans out to
recipients with different scopes/identities — the username line is conditional on
the recipient, not the event. FR-003 (non-admins never see another user's name) is
already enforced by the existing scope gate (`scope != "any"` recipients only get
their own tasks).

**Alternatives rejected**:
- *Reuse the client title logic only*: notifications are built server-side in the
  watcher; the client isn't involved. Rejected.
- *Single shared payload with the username always included*: would leak the owner
  to the owner themselves and complicate the (rare) case; per-subscriber build is
  simpler and correct. Rejected.

## D6 — Statistics aggregation & time bucketing

**Decision**: Two read-only endpoints.
`GET /v1/stats/summary` returns per-visible-user, per-source category counts and
average sizes (SQL `GROUP BY user_id, source, category`, `AVG(size_bytes)` over
non-NULL sizes). `GET /v1/stats/timeseries` returns **daily** counts
(`GROUP BY date(created_at,'unixepoch')`) for the visible scope/source. The
**client** aggregates daily counts into week/month/year/all-time buckets in
`stats-buckets.ts` so switching granularity needs no refetch (SC-004) and bucket
boundaries follow the viewer's local time.

**Rationale**: Daily is the finest useful grain and small (≤366 points/year);
coarser buckets are pure client math. This keeps the server queries trivial and
index-friendly (`idx_download_history_user(user_id, created_at)`), satisfies the
"switch without reload" criterion, and sidesteps server-side timezone handling.

**Role gating**: both endpoints resolve the visible user set from the session —
non-admins are forced to their own `user_id` (mirrors
`EffectiveNotificationScope`/`decorateTasks`); admins/owner may request a specific
user or all. This reuses the exact role model already in place (`is_admin`; owner
= first user).

**Alternatives rejected**: server-side bucketing per granularity (needs a
timezone, refetch per switch); returning raw rows to the client (leaks
cross-user data to non-admins, more payload). Both rejected.

## D7 — No new dependencies; chart is hand-rolled SVG

**Decision**: Render the historical graph as an inline `<svg>` bar chart in
`DownloadsChart.vue`, styled with existing `--app-*` theme tokens, wrapped in
stock Ionic controls (`ion-segment` for source & bucket, `ion-select` for the
admin user picker, `ion-list` for the numeric summary).

**Rationale**: Honors the requester's "no new charting dependency" and the
constitution's minimal-dependency + Ionic-first posture. A bar chart of ≤~60
visible buckets is simple SVG (rects + a couple of axis labels); accessibility is
handled with a text summary alongside. Justified deviation recorded in the plan's
Complexity Tracking.

**Alternatives rejected**: Chart.js / ApexCharts / d3 (runtime dependency + bundle
weight, declined by requester).
