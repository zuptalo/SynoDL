# Contract: `GET /v1/tasks` (extended — added `errorDetail`)

Existing endpoint. **Only change**: each task object MAY carry a new optional `errorDetail` string.
Everything else (request, auth, all other fields, filter/sort done client-side) is unchanged.

## Response (success) — shape

```jsonc
{
  "tasks": [
    {
      "id": "dbid_xxx",
      "name": "Ubuntu 24.04.iso",
      "type": "bt",
      "status": "error",
      "size": 0,
      "downloaded": 0,
      "uploaded": 0,
      "downloadSpeed": 0,
      "uploadSpeed": 0,
      "peers": 0,
      "seeders": 0,
      "createdAt": 1753000000,
      "destination": "movie",
      "errorDetail": "broken_link"   // NEW — present only when DSM supplies status_extra.error_detail
    }
  ],
  "stats": { "downloadSpeed": 0, "uploadSpeed": 0 }
}
```

## Rules

- `errorDetail` is the raw DSM `status_extra.error_detail` keyword, forwarded verbatim (no server-side
  interpretation). Omitted or empty when DSM provides none.
- Backward compatible: existing clients ignore the unknown field; the field is additive.
- The stream (`tasks-stream.md`) emits the identical object shape.

## Backward-compat / non-goals

- No change to which tasks are returned, ordering (client sorts), or any other field.
- No new query parameters.
