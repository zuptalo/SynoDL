# Contract: `synomock` control-endpoint additions

The mock DSM already exposes `/__mock/reset`, `/__mock/seed`, and `/__mock/tick` (a virtual clock;
`downloading` tasks compute progress from `rate` × elapsed virtual time, so ticking advances the
numbers deterministically). This feature needs only ONE data addition — the rest is reused.

## Addition 1 — `errorDetail` on the mock Task / seed shape

Add a field to the mock `Task` (and therefore the `/__mock/seed` wire shape):

```jsonc
// POST /__mock/seed  { "tasks": [ ... ] }
{
  "name": "broken.iso",
  "type": "bt",
  "status": "error",
  "size": 0,
  "rate": 0,
  "errorDetail": "broken_link"   // NEW
}
```

When a task has a non-empty `errorDetail` (or, defensively, whenever `status == "error"`), the mock's
task-list response MUST include it in the DSM-shaped `status_extra`:

```jsonc
{ "id": "...", "title": "...", "status": "error",
  "status_extra": { "error_detail": "broken_link" },
  "additional": { "detail": { ... }, "transfer": { ... } } }
```

This lets server unit tests and e2e assert the reason end-to-end without real hardware.

## Reused — live updates via `/__mock/tick`

No new control needed for streaming tests. An e2e:
1. `POST /__mock/seed` a `downloading` task with `rate > 0`.
2. Load the app (stream connects).
3. `POST /__mock/tick` (advance the virtual clock).
4. Assert the row's progress/speed advanced **without a manual refresh** — proving the SSE push path.

## Optional (only if a status transition is needed) — force a status

If a test needs a mid-stream status change (e.g. `downloading → finished`) that time alone can't
produce, add a minimal control; otherwise omit:

```
POST /__mock/task-status   { "id": "<task id>", "status": "finished" }
```

Kept optional to avoid speculative surface area — prefer `tick`-driven progress where it suffices.

## Non-goals

- No auth/session changes to the mock.
- No `status_extra` fields beyond `error_detail` (e.g. `unzip_progress`) unless a later spec needs them.
