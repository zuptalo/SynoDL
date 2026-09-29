# Contract: admin HTTP API

All routes are under `/v1/library/repair`, stateful mode only, and **admin only**
(`d.requireAdmin`): a non-admin gets `403` from every one, whatever the body. They are
registered inside the same `if d.Stateful` block as the other library routes.

## `GET /v1/library/repair` → `200`

The whole screen in one request (the client polls it every 3 s while a run is active).

```json
{
  "available": true,
  "reason": "",
  "current": {
    "id": "a1b2c3d4e5f6", "kind": "check", "state": "running",
    "startedAt": 1790000000, "startedBy": "Kamran",
    "progress": { "phase": "lookup", "done": 1275, "total": 4017 }
  },
  "plan": {
    "id": "20260929T071341Z-73c844", "checkedAt": 1790003000, "expiresAt": 1790089400,
    "status": "ready", "canApply": true, "canContinue": false, "canUndo": false,
    "summary": { "kind": "check", "ok": true, "check": { "duplicates": 1774, "…": 0 } }
  },
  "latest": { "id": "…", "kind": "apply", "state": "finished", "…": "…",
              "summary": { "kind": "apply", "ok": true, "apply": { "done": 15976, "skipped": 155, "failed": 0 } } },
  "history": [ { "id": "…", "kind": "check", "state": "finished", "startedAt": 0, "finishedAt": 0,
                 "startedBy": "Kamran", "planId": "…", "headline": "1,774 duplicates, 3,970 moves" } ]
}
```

- `available:false` carries `reason` ∈ `not_configured` (no cluster access, or no music
  library configured), `no_image` (the server could not determine its own image; set
  `MUSIC_REPAIR_IMAGE`). The rest of the object is then empty. It is still `200`: the
  section explains itself.
- `current` is `null` when nothing is running. `progress` may be absent (the worker has
  not reported yet).
- `plan` is the most recent check that has a plan id, or `null`.
- `latest` is the newest run that has ended, with its bounded `summary` (an apply's skipped steps with reasons, an undo's counts); only that one run carries a summary — history rows stay small.
- Nothing in it is worker output except the decoded, bounded `summary`.

## `POST /v1/library/repair/check` → `202 {"id": "…"}`

No body. Starts a check (the tool's `plan`).

## `POST /v1/library/repair/apply` → `202 {"id": "…"}`

```json
{ "planId": "20260929T071341Z-73c844", "snapshotAck": true }
```

## `POST /v1/library/repair/undo` → `202 {"id": "…"}`

```json
{ "planId": "20260929T071341Z-73c844" }
```

## Errors

`{ "error": "<code>", "message": "<one plain sentence>" }` — the message is fixed text,
never worker output.

| Status | `error` | When |
|---|---|---|
| 403 | `forbidden` | not an admin |
| 503 | `unavailable` | `available` would be `false` |
| 400 | `bad_request` | malformed body; `planId` not matching `^\d{8}T\d{6}Z-[0-9a-f]{6}$` |
| 400 | `snapshot_required` | apply without `snapshotAck: true` |
| 409 | `busy` | a run is active (adds `startedBy`, `startedAt`; `source: "settings" \| "command_line"`) |
| 409 | `no_plan` | no such plan known to this server (including one made from the command line) |
| 409 | `plan_expired` | the check finished more than 24 h ago |
| 409 | `already_applied` | an apply for that plan already finished |
| 409 | `not_undoable` | nothing applied for that plan, or already undone |
| 502 | `start_failed` | the Job could not be created (the slot is released) |

Every mutating request is authorised, validated, then either starts exactly one Job or
changes nothing.
