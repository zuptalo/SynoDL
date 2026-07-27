# Feature Specification: Per-user notification preferences with task attribution

**Feature Branch**: `feat/1004-notification-prefs`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "Users should be able to configure notifications: for newly added tasks,
completion of a task, or failure of a task — for any user's tasks or just their own. Default should be
only tasks added by themselves."

## User Scenarios

- **US1 (P1)** In Settings, a user chooses which download events notify them — **added / completed /
  failed** — and whether that covers **every user's tasks or only their own**. Default: **own** tasks,
  with completed + failed on and added off.
- **US2 (P1)** Notifications reach only the users whose preferences match the event and scope, honoring
  ownership.

## Functional Requirements

- **FR-001** Per-user preferences (`notify_added`, `notify_completed`, `notify_failed`, `scope` own|any)
  are stored and edited via `GET`/`PUT /v1/notifications/prefs` (self, authenticated). Defaults:
  added=off, completed=on, failed=on, scope=own.
- **FR-002** Since DSM's create call returns no task id, the server records a per-user ownership **claim**
  (creator + title hint) at create time and the push watcher **attributes** a task to its creator when it
  first appears (best-effort name match within a time window). Attribution affects notifications only,
  never access.
- **FR-003** The watcher detects **added / completed / failed** transitions and fans out to each opted-in
  subscription only when that user's prefs enable the event AND their scope covers the task (any user's,
  or their own attributed task).
- **FR-004** An unattributed task notifies only users whose scope is "any" (own-scope users can't be
  confirmed as the owner).
- **FR-005** Instance-level notices (app updates) continue to reach every opted-in device regardless of
  per-task prefs.

## Credential-Safety / Boundary

No new secrets. Claims store only a user id + a task title hint (not URIs/credentials). Preferences are
self-service (a user reads/writes only their own). The watcher still logs nothing sensitive.

## Testing

Server: watcher tests for own vs any scope, unattributed handling, added/completed/failed events, and
prune; store repos via the watcher; prefs handler (defaults, update, scope coercion, auth); titleHint
derivation. Client UI is stateful-only (not in the stateless e2e harness).
