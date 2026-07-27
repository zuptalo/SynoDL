# Feature Specification: Destination overhaul — cancel, default, favorites, and subfolder creation

**Feature Branch**: `feat/1006-destinations`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator requests: "Create new subfolders inside folders we have access to and store downloads
there; cancel out of changing destination; set up a default destination; add up to 4 quickly selectable
popular folders (movie, music, tv-show…); and easily create a subfolder inside a selected favorite and
make it the destination."

## User Scenarios

- **US1 (P1)** In the folder picker, tap **New folder**, name it, and it's created under the current
  folder and opened so **Select** stores downloads there. A user may only create folders where they're
  allowed to download.
- **US2 (P1)** **Cancel** closes the picker without changing the destination.
- **US3 (P2)** Mark a folder as the **default**; new tasks pre-fill that destination.
- **US4 (P2)** Mark up to **4 favorite** folders; they appear as one-tap **chips** in the new-task sheet.

## Functional Requirements

- **FR-001** Add `SYNO.FileStation.CreateFolder` to the proxy allowlist and a `POST /v1/fs/folder`
  endpoint (parent path + name → the created folder).
- **FR-002** In stateful mode the endpoint MUST enforce the SAME per-user create ACL as adding a task to
  that path (`authz.AllowedForCreate`); a folder name MUST be a single segment (no separators/traversal);
  creating a new top-level share is refused.
- **FR-003** The folder picker MUST have a Cancel action that dismisses without emitting a pick.
- **FR-004** The folder picker MUST offer New folder (create + drill into it), and per-folder "Favorite"
  and "Set as default" actions.
- **FR-005** A persisted default destination MUST pre-fill the new-task destination.
- **FR-006** Up to 4 favorite folders MUST be persisted and shown as quick-select chips in the new-task
  sheet; tapping one sets the destination. Preferences persist in IndexedDB (offline-first).

## Credential-Safety / Boundary

Adds exactly one allowlisted DSM API (CreateFolder); no new secret storage. The create ACL reuses the
existing per-user grant checks server-side — the UI never gates access on its own. Folder names are
validated to a single segment to prevent traversal.

## Testing

Server: syno CreateFolder (httptest mock) + handler ACL tests (admin, granted, denied, bad input).
E2E: cancel, create-subfolder-then-select, favorite-then-quick-select.
