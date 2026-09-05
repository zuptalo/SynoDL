# Feature Specification: Keep knowing what is on the NAS

**Feature Branch**: `feat/0011-keep-knowing-what-nas`

**Created**: 2026-09-05

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "let's go with nas inventory and also make sure the downloaded version in owned it is marked correctly for series and movies"

## Overview

SynoDL already works out what a user already has, by reading the download
sources' configured movie and TV parent folders on the NAS. That reading lives
only in memory, and it is thrown away constantly: every deploy restarts the
container, and any moment the NAS is briefly unreachable the reading is replaced
with an empty one. The visible result is that the "you already have this"
markers — the badge in Discover, the seasons breakdown, the marked download
option — disappear, and come back only when someone browses and waits for the
NAS again.

This feature keeps that reading. It is written to the same store that already
holds the rest of the app's state, refreshed on a schedule in the background and
immediately after a download finishes, and read from the store on start-up. The
user-visible change is that the markers are simply there — after a deploy, after
a NAS blip, and without the first person to open a title paying for the read.

## User Scenarios & Testing

### User Story 1 - The markers survive a restart (Priority: P1)

A user opens Discover moments after the operator deployed a new version. The
titles they already have are marked, as they were before the deploy.

**Why this priority**: this is the failure the user actually reported. Deploys
are frequent, and each one blanked the feature until someone warmed it again.

**Acceptance**:
1. Given a previous scan is stored, when the server restarts, then the first
   Discover page marks owned titles without any NAS read having happened first.
2. Given no scan has ever been stored, when the server starts, then Discover
   behaves exactly as it does today (no markers until the first scan lands).

### User Story 2 - A NAS blip no longer blanks ownership (Priority: P1)

The NAS is briefly unreachable. Discover keeps marking what the user has,
based on the last good reading, rather than reporting that they own nothing.

**Acceptance**:
1. Given a stored scan, when the NAS cannot be reached, then markers continue
   to show and no title is reported as absent purely because of the failure.
2. Given no stored scan, when the NAS cannot be reached, then no markers appear
   and browsing is unaffected (unchanged from today).

### User Story 3 - Opening a title doesn't wait on the NAS (Priority: P2)

Opening a title the user owns shows its seasons and marks the version they have
immediately, from the stored reading, while a fresh read happens in the
background.

### User Story 4 - A finished download shows up on its own (Priority: P2)

When a download finishes, the title it belongs to is re-read without anyone
having to browse to it, so the next look already reflects it.

### Edge Cases

- A parent folder is removed from a source's configuration: its stored rows go
  away with it, so nothing is answered for a folder no longer configured.
- A title folder is deleted on the NAS: the stored reading for it is removed on
  the next scan of its parent, not kept indefinitely.
- The store is empty or the scan has never succeeded: identical to today.
- A very large library: a scan cycle refreshes a bounded number of title
  folders, oldest reading first, so the NAS is never flooded.
- Two instances against one store: writes are last-write-wins per folder; a
  reading is a fact about the NAS, not about the instance that read it.

## Requirements

### Functional Requirements

- **FR-001**: The folder listing of each configured parent MUST be persisted.
- **FR-002**: What a title folder was found to contain — whether it holds video,
  which seasons and episodes, and the release tokens and file identities read
  out of the names — MUST be persisted.
- **FR-003**: On start-up the stored reading MUST be loaded before any NAS read,
  so markers are available immediately.
- **FR-004**: A failed NAS read MUST fall back to the stored reading rather than
  to an empty one.
- **FR-005**: A background scan MUST refresh the parent listings on a schedule.
- **FR-006**: The same scan MUST refresh title-folder readings, bounded per
  cycle and oldest-first, so a large library converges without flooding the NAS.
- **FR-007**: A title folder MUST be re-read promptly after a download into it
  finishes.
- **FR-008**: A title folder MUST be re-read promptly after a send into it.
- **FR-009**: Stored rows for a parent that is no longer configured, and for a
  title folder no longer present under a configured parent, MUST be removed.
- **FR-010**: A stored reading served while a fresh read is in flight MUST NOT
  delay the response.
- **FR-011**: Nothing beyond the facts in FR-001/FR-002 may be stored: no file
  contents, no file sizes, no paths outside the configured parents.
- **FR-012**: No folder or file name may reach logs, errors, or metrics — the
  existing rule, restated because there is now a write path too.

### Key Entities

- **Parent listing** — a configured parent path, the folder names read under it,
  whether it serves movies or TV, and when it was read.
- **Folder reading** — one title folder: whether it holds video, the seasons and
  episodes found, the release tokens and file identities, and when it was read.

## Success Criteria

- **SC-001**: After a restart, the first Discover page shows ownership markers
  with no NAS round-trip on the request path.
- **SC-002**: With the NAS unreachable, ownership markers remain for as long as
  a stored reading exists.
- **SC-003**: Opening an already-read title returns its seasons without waiting
  on the NAS.
- **SC-004**: A finished download is reflected without the user browsing to it.
- **SC-005**: A scan cycle issues a bounded number of NAS reads regardless of
  library size.

## Credential-Safety Impact

- **New: NAS-derived facts are written at rest.** Until now the reading existed
  only in memory. What is written is exactly the shape already held in memory —
  parent path, folder name, a presence flag, season and episode numbers, release
  tokens, and file identity keys. No file contents, no sizes, no full paths.
- **No new DSM API.** `SYNO.FileStation.List` is already allowlisted and both
  the folder and file listings are already in use; nothing widens.
- **Instance-wide, unchanged.** The reading is about the operator's own
  configured folders and is the same for every user, exactly as the memory cache
  it replaces. Which user may SEND into a folder is still governed by their
  folder grants; this changes nothing about that.
- **No secrets.** No credential, session id, or task URI is involved.
- **Still never logged.** Folder and file names remain absent from logs, error
  strings, and metrics.
- **Removable.** Rows are deleted when their parent stops being configured, so
  the store never retains a reading of a folder the operator has disconnected.

## Assumptions

- Scan cadence and per-cycle bound are operator-invisible constants, tuned in
  code, not configuration.
- Eventual consistency is acceptable: a large library converges over several
  cycles rather than in one.
