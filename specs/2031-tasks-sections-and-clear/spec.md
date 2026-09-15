# Feature Specification: The newest block on top, and one clear for both systems

**Feature Branch**: `feat/2031-tasks-sections-and-clear`

**Created**: 2026-09-15

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "let's make it so the section with the most recently added task shows
up at the top, it being youtube, or download or upload" — and, on finding that
Clear finished left saved YouTube downloads behind: "add it".

## Overview

The Tasks list holds three kinds of thing from three separate systems: uploads,
YouTube downloads, and NAS downloads. Two things about that were wrong.

**The order was hard-coded.** Uploads, then YouTube, then NAS. So whatever you
had just started could sit underneath whatever you did last week: send a film
from Discover while a playlist is saving and the new one is below twelve
finished tracks.

**Clear finished only cleared one of the three.** It is wired to the NAS list and
calls Download Station's delete; the YouTube downloads live in a different store
behind their own endpoint. A screen of saved tracks could only be cleared one row
at a time, and the button's count said so while looking like it meant everything.

## User Scenarios & Testing

### User Story 1 - The newest thing is where you look (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a YouTube download newer than every NAS download, **When** the list
   is shown, **Then** the YouTube block is first.
2. **Given** an upload started after that, **When** it is, **Then** the uploads
   block is first.
3. **Given** a NAS download started after THAT, **When** it is, **Then** the
   downloads block is first.
4. **Given** a block moves, **When** it does, **Then** the others are still
   shown — reordering is not hiding.
5. **Given** a block is empty, **When** it is, **Then** it is not rendered and
   does not take a position.

---

### User Story 2 - Clear finished clears what is finished (Priority: P1)

**Acceptance Scenarios**:

1. **Given** saved YouTube downloads and finished NAS downloads, **When** Clear
   finished is used, **Then** both go.
2. **Given** the confirmation, **When** it is shown, **Then** it says that a
   cleared YouTube download will be fetched again if its playlist or channel is
   re-run.
3. **Given** specific NAS tasks are SELECTED, **When** Clear finished is used on
   that selection, **Then** only those are cleared — a selection is a selection.
4. **Given** anything is cleared, **When** it is, **Then** no media file is
   touched.

---

### Edge Cases

- **A group and its items.** A group's items are removed by the group's own
  cascade, so counting them separately would promise a number it then does not
  delete.
- **Two things started in the same second.** Timestamps are unix seconds, so a
  tie is possible. It must resolve to a fixed order rather than flicker.
- **Nothing anywhere.** The empty state has to mean "nothing at all", not "no NAS
  downloads" — it used to say the latter while a YouTube list sat above it, and
  with the blocks reordered it could also have landed underneath them.

## Requirements

- **FR-001**: The block holding the most recently added item MUST be first.
- **FR-002**: Ordering MUST use the NEWEST member of each block, not an average
  or a total — the question being asked is "what about the thing I just did".
- **FR-003**: A block with no items, or no usable timestamps, MUST sort last.
- **FR-004**: A tie MUST resolve to a fixed order, so the list cannot reorder
  itself while nothing has changed.
- **FR-005**: Timestamps MUST be compared in one unit. NAS tasks and the server
  already speak unix seconds; uploads MUST be stamped in the same unit.
- **FR-006**: Clear finished MUST remove finished NAS downloads AND saved
  YouTube downloads.
- **FR-007**: It MUST count and delete the same things — whole downloads and
  whole groups, never a group's items separately.
- **FR-008**: Its confirmation MUST say that clearing a saved YouTube download
  means it is fetched again on a re-run. That is not obvious, and it is not about
  files: the completed record IS the "already held" memory.
- **FR-009**: Clearing from a SELECTION MUST clear only the selection.
- **FR-010**: No media file may be deleted by any of this.
- **FR-011**: The empty state MUST mean nothing in any block.

## Success Criteria

- **SC-001**: With a YouTube download, then an upload, then a NAS download each
  added after the last, the top heading follows each in turn.
- **SC-002**: Clearing removes saved YouTube rows as well as finished NAS ones.
- **SC-003**: The confirmation names the re-fetch consequence.

## Credential-Safety Impact

- **None.** Ordering is presentational. The clear uses the two existing delete
  endpoints, each already scoped to the requesting user — the YouTube one keeps
  ownership in its DELETE's WHERE clause, so "not yours" and "not there" stay
  indistinguishable.

## Assumptions

- "Finished" for a YouTube download means `completed` — the state the UI shows as
  *saved*. Failed downloads are deliberately left, since a failure is something
  to look at or retry rather than tidy away.
