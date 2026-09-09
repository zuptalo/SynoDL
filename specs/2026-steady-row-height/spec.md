# Feature Specification: A row that stays the height it was

**Feature Branch**: `fix/2026-steady-row-height`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Reported from use, with screenshots taken a moment apart: in an
expanded playlist, the same list of tracks sits at two different sets of
positions depending on which of them happen to be showing a progress bar.

## Overview

A progress bar is drawn only while a download is running and only when a reading
is actually available — which is right, and spec 0013 argued for it: a bar pinned
at zero reads as a stalled download, so showing nothing is the honest answer.

The bar's SPACE was conditional too, and that is the defect. The bar is three
pixels tall, so a row grew by three pixels when a reading arrived and shrank by
three when the download finished, moving every row beneath it each time.

On a single download nobody would notice. On an expanded playlist it is
continuous: items finish one after another, each one shortening its own row, and
the list creeps upward under a finger that is trying to tap something. The two
screenshots that reported this differ by exactly one track's worth of shift.

The same conditional bar is in the NAS task row and the upload row, which sit in
the same list. Fixing only the YouTube row would have made the three kinds of row
disagree about their own height — the opposite of what spec 0013 was careful to
line up.

## User Scenarios & Testing

### User Story 1 - Watching a playlist download (Priority: P1)

Someone opens a playlist that is part-way through and watches its tracks finish.
The rows stay where they are.

**Independent Test**: put one download in a running state with no reading, note
the row's height, let a reading arrive, and confirm the height is unchanged.

**Acceptance Scenarios**:

1. **Given** a running download with no reading yet, **When** a reading arrives
   and the bar appears, **Then** the row is the height it already was.
2. **Given** a running download showing a bar, **When** it finishes and the bar
   goes, **Then** the row is the height it already was.
3. **Given** a list mixing NAS downloads, uploads and YouTube downloads, **When**
   any of them starts or finishes, **Then** no other row moves.

---

### Edge Cases

- **A download that never reports a reading.** It shows no bar for its whole
  life, and its row still measures the same as one that does — otherwise the
  fix would only have moved the inconsistency.
- **The bar is still absent, not merely invisible.** "We cannot see" must stay
  distinguishable from "zero percent"; reserving space must not put a bar on
  screen.

## Requirements

### Functional Requirements

- **FR-001**: A download row MUST be the same height whether or not it is
  currently showing a progress bar.
- **FR-002**: The bar MUST still be absent when there is nothing to report — no
  bar at zero, and no faint or empty track drawn in its place.
- **FR-003**: The NAS download row, the upload row and the YouTube download row
  MUST reserve the same space, so a mixed list stays flush.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A row measures identically across the appearance of its bar.
- **SC-002**: A regression test fails if the bar's space is ever made conditional
  again.

## Assumptions

- Three pixels of unused space on a row that will never show a bar is a cheaper
  cost than the movement, and is not visible on its own.
