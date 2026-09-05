# Feature Specification: Forget content that has left the NAS

**Feature Branch**: `feat/1029-forget-content-left-nas`

**Created**: 2026-09-06

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User description: "we need to have some scheduled scans and see if anything has been removed from the nas folders and update our database accordingly as well, if a folder doesn't exist anymore or the folder doesn't contain any movie file, then we can consider it as gone and we can delete it's data from the database"

## Overview

Spec 0011 gave SynoDL a scheduled scan and a stored reading of what is on the
NAS. That reading already forgets a title folder that has disappeared from a
configured parent. Two things it does not do:

1. A folder that still EXISTS but no longer holds any video is remembered as
   though the content were there. Emptying a folder without removing it is the
   ordinary result of deleting the files from a media server.
2. Nothing ever removes the record of WHAT WAS SENT to a folder. That record
   outlives the content indefinitely, so the database accumulates rows for
   titles the user deleted months ago.

This makes the scheduled scan reconcile both: a title folder that is gone, or
that holds no video, is treated as gone, and the data SynoDL holds about it is
removed.

## Why this needs care

The record of what was sent is the ONLY thing that knows which version a user
downloaded. A library renamed for a media server carries no release information
in its file names, so nothing can recover it once deleted (spec 0010, spec
2015). Deleting it on a single reading is therefore irreversible, and several
ordinary situations look exactly like "empty":

- a download in flight, before the file appears
- an archive that has not been extracted yet
- a folder the media server is moving or reorganising
- a read that partly succeeded

So removal is deliberately slow: a folder must be seen as gone on repeated scans
over a grace period before anything is deleted, and a folder being downloaded
into is never considered gone.

## User Scenarios & Testing

### User Story 1 - Deleting a title clears its record (Priority: P1)

A user deletes a film from the NAS. After the grace period, SynoDL no longer
holds a record of what was downloaded there.

**Acceptance**:
1. Given a recorded download whose folder no longer exists, when scans have seen
   it gone for the grace period, then its record is deleted.
2. Given a recorded download whose folder exists but holds no video, the same.
3. Given the folder is restored before the grace period elapses, then the record
   is kept and its missing mark cleared.

### User Story 2 - A rename is not a removal (Priority: P1)

A media server renames a folder after the download lands. That must never read
as a removal.

**Why this priority**: a folder is renamed on essentially every download in a
managed library. Getting this wrong deletes the entire record set at once.

**Acceptance**:
1. Given a folder renamed from `Title` to `Title (2026)`, when the scan runs,
   then the record is matched to the renamed folder and is not marked missing.

### User Story 3 - A NAS that cannot be read changes nothing (Priority: P1)

**Acceptance**:
1. Given the NAS is unreachable, when a scan runs, then no record is marked
   missing and none is deleted.
2. Given a folder whose contents could not be read, then it is not treated as
   empty.

### User Story 4 - An unfinished download is not gone (Priority: P2)

**Acceptance**:
1. Given a folder that is the destination of a task currently downloading, when
   the scan runs, then it is not marked missing even though it holds no video.
2. Given a folder whose task is PAUSED, the same — it will fill when the download
   resumes.
3. Given the task is removed from Download Station, then the folder is judged on
   its contents like any other.

### Edge Cases

- A folder emptied and refilled within the grace period: the mark is cleared and
  nothing is deleted.
- A series whose season folders are removed one at a time: the title folder is
  gone only when it holds no video at all.
- Two records whose folders resolve to the same title: each is judged on its own
  destination.

## Requirements

### Functional Requirements

- **FR-001**: The scheduled scan MUST reconcile every recorded download against
  the current reading of the NAS on each cycle.
- **FR-002**: A recorded download whose title folder is absent from every
  configured parent MUST be considered gone.
- **FR-003**: A recorded download whose title folder holds no video MUST be
  considered gone.
- **FR-004**: A folder MUST be matched to its record by the same name comparison
  the library index uses, so a renamed folder is not read as a removal
  (spec 2015).
- **FR-005**: A record first seen gone MUST be marked with the time it was first
  seen gone, and MUST NOT be deleted on that cycle.
- **FR-006**: A record seen gone continuously for the grace period MUST be
  deleted.
- **FR-007**: A record whose folder is seen present again MUST have its missing
  mark cleared.
- **FR-008**: A folder that could not be READ — as opposed to one read and found
  empty — MUST NOT be marked missing.
- **FR-009**: A folder that is the destination of a task that has NOT finished —
  paused, waiting and errored included, not only actively downloading — MUST NOT
  be marked missing. A paused download's folder is empty now and will not be when
  it resumes.
- **FR-010**: Nothing may be deleted when the scan could not read the NAS at all.
- **FR-011**: The per-user download HISTORY MUST NOT be touched. It is an
  append-only statistics and quota-accounting log; deleting from it would
  retroactively rewrite what a user downloaded and how much of their allowance
  they used.
- **FR-012**: No folder or file name may reach logs, errors, or metrics.

### Key Entities

- **Recorded download** — one title folder SynoDL sent content to, what version
  was sent, and (new) when it was first seen gone.

## Success Criteria

- **SC-001**: A record whose folder is deleted is gone from the database after
  the grace period, without any user action.
- **SC-002**: A record whose folder is emptied is treated identically.
- **SC-003**: No record is deleted for a folder that was merely renamed.
- **SC-004**: No record is deleted while the NAS is unreachable.
- **SC-005**: Reconciliation costs no additional NAS reads beyond the scan the
  instance already performs.

## Assumptions

- The grace period is an operator-invisible constant, tuned in code.
- "Holds video" means the same thing it already means to ownership: at least one
  file the library recognises as video, anywhere under the title folder.

## Credential-Safety Impact

None beyond spec 0011's. No new DSM API and no new NAS read: reconciliation uses
the reading the scan already produced. Nothing new is stored — one timestamp is
added to a row that already exists — and folder names stay out of logs.
