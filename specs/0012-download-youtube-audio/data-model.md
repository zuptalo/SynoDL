# Data Model: Save YouTube music and music videos to the library

**Spec**: [spec.md](./spec.md) | **Date**: 2026-09-06

The governing idea: **the cluster owns lifecycle, SynoDL owns only regret.**
In-flight state is derived, never stored. The one thing written at rest is a
failed download, because that is the only outcome whose evidence would otherwise
disappear (a success leaves the file).

## 1. Worker Job (owned by the orchestrator, not stored)

A Job is created per request and is the single source of truth for its state.
Everything the UI shows about an in-flight download is read back from a
label-selector LIST.

**Labels** (all required; the selector is `managed-by` + `kind`):

| Label | Value | Purpose |
|---|---|---|
| `app.kubernetes.io/managed-by` | `synodl` | Never touch anything we did not create |
| `synodl.io/kind` | `ytdl` | Distinguishes these from any future worker type |
| `synodl.io/request-id` | opaque id | Correlates Job ↔ client row ↔ failure record |
| `synodl.io/mode` | `music` \| `music-video` | Which library it targets |
| `synodl.io/scope` | `single` \| `playlist` \| `channel` | Display only |

**Annotations** (not labels — values are unconstrained and may be long):

| Annotation | Value |
|---|---|
| `synodl.io/source-url` | the submitted link, echoed back for display |
| `synodl.io/submitted-by` | the SynoDL user id that requested it |

**State derivation** — the whole of FR-017, and the only place states are
decided:

| Job status | Reported state |
|---|---|
| exists, `active == 0`, no terminal condition | `scheduled` |
| `active > 0` | `started` |
| condition `Complete`, or `succeeded > 0` | `completed` |
| condition `Failed`, or `failed > 0` | `failed` |
| deadline exceeded (`reason: DeadlineExceeded`) | `failed` |
| absent from the LIST, and a failure record exists | `failed` |
| absent from the LIST, and no failure record | not shown (swept after success) |

The last two rows are what make FR-018 hold: a vanished Job is not assumed
successful. It is only ever reported as completed on positive evidence.

## 2. Failure record (stored — migration `0020`)

The single addition to the SQLite store. Written when a Job reaches `failed`;
never written for a success.

```sql
-- 0020 — remember downloads that failed (spec 0012), so an unsuccessful
-- outcome survives the cluster sweeping its Job away. Successes are NOT
-- recorded: the saved files are their own record.
CREATE TABLE ytdl_failures (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id   TEXT NOT NULL UNIQUE,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    source_url   TEXT NOT NULL,
    mode         TEXT NOT NULL,
    scope        TEXT NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    failed_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_ytdl_failures_failed_at ON ytdl_failures (failed_at DESC);
```

**Rules**

- `request_id` is unique, so observing the same failed Job twice is idempotent —
  important because state is polled, not evented.
- `user_id` is `ON DELETE SET NULL`: deleting a user must not erase the operator's
  record that a download failed, but must not retain a dangling reference either.
- `reason` is a short, already-sanitised string. It MUST NOT carry a command
  line, a filesystem path, or raw worker output (Principle III).
- **Retention (FR-024a)**: user-dismissable individually, and pruned to a bounded
  number of most-recent rows on write, so the table cannot grow without limit.

## 3. Media library (operator configuration, not per user)

Two instance-wide destinations, resolved from config, never from a request:

| Field | Notes |
|---|---|
| `mode` | `music` or `music-video` — the key |
| `mount path` | where the worker mounts it; the server never mounts either |
| `owner uid/gid` | applied to the worker so files are not root-owned |

A request carries a mode, never a path. There is no code path from user input to
a destination directory, which is what keeps FR-006 structurally true rather than
merely tested.

## 4. Client task type (discriminated union)

`Task` gains a discriminator so one list can hold both kinds without either
pretending to be the other:

```ts
source: 'nas' | 'ytdl'   // absent ⇒ 'nas', so existing rows keep working
```

For a `ytdl` row: `status` is one of the four states; `size`, `downloaded`,
`downloadSpeed`, `uploadSpeed`, `peers`, and `seeders` are absent — not zero.
Rendering must branch on `source`, never on "is the number zero", so a real
0-byte NAS task is not mistaken for a YouTube row.

**Action matrix (FR-027, FR-028)**

| Action | `nas` | `ytdl` |
|---|---|---|
| Pause / Resume | ✅ | ❌ never offered |
| Delete / dismiss | ✅ | ✅ (dismiss only; nothing is deleted from the library) |
| Copy source URL | ✅ | ✅ |
| Re-download | ✅ | ✅ (re-submits the link) |
| Bulk select | ✅ | ✅ — but a bulk NAS action MUST skip `ytdl` rows |

Bulk actions filter by `source` before dispatching. A NAS-only action applied to
a mixed selection acts on the NAS rows and reports how many were skipped.
