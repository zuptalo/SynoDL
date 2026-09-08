# Data Model: spec 0013

**Phase 1 output** · Entities, their fields, their rules, and the state machine.

## Design rule this model obeys

Principle III permits a durable record of *requested and finished* work and
forbids mirroring *in-flight worker state*. The split below is the whole design:

| Fact | Where it lives | Why |
|---|---|---|
| What was asked for (link, mode, scope, who, when) | SQLite, durable | It is the request, and it outlives every worker |
| What the source said it is (title, uploader, artwork) | SQLite, durable | Learned once at submission; re-fetching it later would be a second network dependency for a fixed fact |
| Queue position, before admission | SQLite, durable | There is no worker yet, so nothing is being mirrored (FR-023) |
| Live state while a worker exists | Derived from the orchestrator | Principle III; a mirror drifts across a restart |
| Live progress percentage | Server memory, lost on restart | A cache of a reading, not a record. FR-013f |
| Terminal outcome, reason, lyrics, language, timestamps | SQLite, durable, written once | Captured while the worker's output is still readable (FR-013g) |

## Entity: Download

One row per download. A group and its items are all Downloads, distinguished by
`kind` and joined by `parent_id`.

| Field | Type | Notes |
|---|---|---|
| `request_id` | TEXT PK | Correlation handle, never a capability (0012's existing scheme) |
| `parent_id` | TEXT NULL → Download | Set on an item; NULL on a single download and on a group |
| `kind` | TEXT | `single` \| `group` \| `item` |
| `user_id` | INTEGER NULL → users | `ON DELETE SET NULL` — records outlive the account (R10, FR-006d) |
| `source_url` | TEXT | Normalised by `Classify`, always allowlisted (FR-016a) |
| `video_id` | TEXT | Stable identity for the already-held check (FR-016b); empty for a group |
| `mode` | TEXT | `music` \| `music-video` — selects the library, never a path |
| `scope` | TEXT | `single` \| `playlist` \| `channel` |
| `state` | TEXT | See the state machine below |
| `title`, `uploader`, `artwork` | TEXT | Best-effort, empty is normal (0012's `Description`) |
| `group_name` | TEXT | The playlist or channel name, sanitised (FR-038a/b), passed to an item's worker for album naming (FR-036/037) |
| `origin` | TEXT | `direct` \| `expanded` — drives fair-share ordering (FR-022c) |
| `has_lyrics` | INTEGER | 0/1/NULL-unknown, captured per FR-013g |
| `lyrics_lang` | TEXT | Language of the companion file, when one was written |
| `reason` | TEXT | Plain language only — never a path, command or raw output (FR-032) |
| `attempts` | INTEGER | Incremented by retry; the row stays one row (FR-029) |
| `queue_seq` | INTEGER | Monotonic submission order; the tiebreak inside fair-share |
| `created_at` | INTEGER | Unix seconds (FR-003) |
| `finished_at` | INTEGER NULL | NULL until final — absent, not zero (FR-033) |

**Indexes**: `(user_id, created_at DESC)` for the paged list (FR-006a);
`(parent_id)` for a group's items; `(state)` for the reconciler's admission
query; `(video_id, mode)` for the already-held check (FR-020).

**Rules**

- A `group` never has a worker of its own once expansion finishes; its state is
  derived from its items' states.
- An `item` always has a `parent_id`; a `single` never does.
- `video_id` + `mode` is the already-held key. A row that is `completed` makes
  that pair held; dismissal removes the row and so un-holds it (FR-020a).
- Nothing here is encrypted: these are user-chosen public URLs and public
  metadata, explicitly not secrets (Credential-Safety Impact §4). They are still
  user data — out of logs, owner-and-admin visible, removable.

## Entity: WorkerReading (memory only)

Not a table. The reconciler's latest reading per running download.

| Field | Notes |
|---|---|
| `percent` | Monotonic-clamped in the server, so every viewer sees the same corrected value (R3) |
| `phase` | Which pass of a two-stream fetch is running |
| `read_at` | Staleness guard: a reading older than a bound is dropped, and the row shows no percentage rather than a stale one (FR-013) |

Lost on restart by design. Losing it must never change a state or fail a
download (FR-013).

## State machine (FR-013a–FR-013d)

```
                    ┌─────────────┐
  group submitted → │  resolving  │ ─── expansion failed ──→ failed
                    └──────┬──────┘
                           │ entries found → one item per entry
                           ▼
  single submitted → ┌──────────┐
                     │  queued  │ ←──────── retry (explicit only) ───┐
                     └────┬─────┘                                    │
                          │ admitted (fair-share, ≤ limit)           │
                          ▼                                          │
                     ┌───────────┐                                   │
                     │ scheduled │  Job created, no pod running yet   │
                     └────┬──────┘                                   │
                          ▼                                          │
                    ┌─────────────┐                                  │
                    │ downloading │  progress applies ONLY here      │
                    └──┬───────┬──┘                                  │
                       ▼       ▼                                     │
                ┌───────────┐ ┌────────┐                             │
                │ completed │ │ failed │ ────────────────────────────┘
                └───────────┘ └────────┘
                     final         final
```

- Failure is checked before success at every step — reporting an unsuccessful
  download as completed is the one outcome 0012 FR-018 forbids, and this spec
  does not relax it.
- `resolving`, `queued` and `scheduled` must each be able to reach a final state
  unattended (FR-013d), so each carries a bound: expansion has a deadline, an
  admitted Job has the existing `activeDeadlineSeconds`, and a `scheduled` Job
  whose pod never appears eventually fails rather than waiting forever.
- `failed → queued` is the ONLY edge out of a final state, and only on an
  explicit user action (FR-027).

## Group state derivation

A group is `resolving` while expansion runs; afterwards its state is a function
of its items: `downloading` while any item is not final, then `completed` if
every item completed, otherwise `failed`. Its displayed counts — saved, failed,
remaining — are the aggregate FR-019 requires, and its single notification
(FR-025c) fires on the transition to a final state.

## Migration

One appended migration (R10). Every statement `IF NOT EXISTS`; the backfill from
`ytdl_failures` is `INSERT OR IGNORE`, so the whole migration is re-runnable and
the spec 1031 drift repair still works. `ytdl_failures` is left in place and
stops being read.
