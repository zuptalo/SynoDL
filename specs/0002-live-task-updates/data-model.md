# Data Model: Live task updates, failure reasons, detail view

This feature is **additive** — it adds one field and two transport/view concepts. It changes no
existing field and persists nothing new.

## Task (extended)

The existing flat `Task` DTO (server `internal/syno` → wire → client `src/types/task.ts`) gains ONE
optional field. Everything else is unchanged.

| Field | Type | Source | Notes |
|---|---|---|---|
| id | string | DSM `id` | unchanged |
| name | string | DSM `title` | unchanged |
| type | string | DSM `type` | unchanged |
| status | string | DSM `status` | unchanged (`downloading`, `paused`, `finished`, `seeding`, `error`, …) |
| size | number | DSM `size` | unchanged |
| downloaded | number | DSM `additional.transfer.size_downloaded` | unchanged |
| uploaded | number | DSM `additional.transfer.size_uploaded` | unchanged |
| downloadSpeed | number | DSM `additional.transfer.speed_download` | unchanged |
| uploadSpeed | number | DSM `additional.transfer.speed_upload` | unchanged |
| peers | number | DSM `additional.detail.connected_peers` | unchanged |
| seeders | number | DSM `additional.detail.connected_seeders` | unchanged |
| createdAt | number | DSM `additional.detail.create_time` | unchanged (unix seconds) |
| destination | string | DSM `additional.detail.destination` | unchanged |
| **errorDetail** | **string (optional)** | **DSM `status_extra.error_detail`** | **NEW.** Empty/absent when not applicable. Raw DSM keyword (e.g. `broken_link`); mapped to friendly text on the client. Returned by BOTH `/v1/tasks` and the stream. |

**Validation / rules**: `errorDetail` is passed through verbatim from DSM (a short keyword), never
parsed server-side beyond JSON. The client treats an unknown/empty value as "no specific reason" and
shows a generic message; it is only *displayed* for tasks whose `status` is `error`.

## Task update snapshot (streamed)

Not a stored entity — the payload of one SSE `data:` event and the existing `/v1/tasks` body. Shape is
unchanged from today's task-list response:

```jsonc
{ "tasks": [ Task, ... ], "stats": { "downloadSpeed": number, "uploadSpeed": number } }
```

The stream emits this whole snapshot (not deltas) each tick; the client replaces its reactive `tasks`
+ `stats` wholesale, exactly as `refresh()` does today — so filter/sort/search and the detail view all
keep working against the same reactive source.

## Failure reason (client-derived, not stored)

A pure function `reasonFor(errorDetail: string): string` in `src/services/task-error.ts`:
- Known keyword → friendly sentence (table in research.md D6).
- Unknown non-empty keyword or empty → generic "Download failed".
- No side effects, no I/O → unit-tested, on the vitest coverage allowlist.

## Client reactive/session state (unchanged stores)

- No new IndexedDB object store; `DB_VERSION` is **not** bumped (Principle IV).
- `useTasks` internal state gains transport bookkeeping only (an `AbortController`, a
  `mode: 'stream' | 'poll'` indicator, reconnect backoff timers) — in-memory, not persisted.
