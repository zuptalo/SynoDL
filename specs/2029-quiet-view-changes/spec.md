# Feature Specification: Only a typed search offers to be called off

**Feature Branch**: `fix/2029-quiet-view-changes`

**Created**: 2026-09-10

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "changing the source and filter and sorting still shows the old
spinning animation with X Cancel button on the contents, we should only show the
loading progresbar in these scenarios"

## Overview

Spec 1041 gave a running search a cancel. Spec 2028 took it away from a pull
(the ring carries its own) and from paging (nothing is being taken away, so
there is nothing to take back). What is left is the case the reader has now
watched in practice: changing the source, a filter or the sort still parks a
spinner and a Cancel button over the grid.

The same argument that removed it from paging removes it here. Changing the sort
is a flick of a control that answers in well under a second; the grid underneath
is about to be reordered, not replaced by something the reader has to wait for.
A spinner over the content is a heavier report than the event deserves, and it
covers the very thing it is reporting on. The indeterminate bar in the header
already says a request is out, and it costs no layout.

A TYPED query is different, and that is the one this keeps. It is the only
search a reader composes rather than picks: it can be long, it can be wrong, it
can go to a slow full-text endpoint, and abandoning it has to put the old view
and the old query back (spec 1041 FR-009). That is worth an explicit control.

## User Scenarios & Testing

### User Story 1 - Changing the view is quiet (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the reader changes the sort, a filter, or the source, **When** the
   search runs, **Then** the header's progress bar reports it and nothing is
   drawn over the grid.
2. **Given** any of those is running, **When** the screen is watched, **Then** no
   Cancel control appears.
3. **Given** the search finishes, **When** it does, **Then** the bar returns to
   idle without the header changing height.

---

### User Story 2 - A typed query still offers a way out (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the reader types a query, **When** the search runs, **Then** the
   spinner and Cancel appear as before.
2. **Given** it is called off, **When** it is, **Then** the previous results and
   the previous query come back.
3. **Given** the block is on screen, **When** it appears, **Then** the grid does
   not move.

---

### Edge Cases

- **A pull-to-refresh.** Already covered by spec 2028: the ring carries its own
  cancel, and the floating block must not double it.
- **Paging.** Already quiet (spec 2028 FR-013); this must not re-arm it.
- **A view change while a typed search is in flight.** The later search decides
  what is shown, so the block must follow the search that is actually running
  rather than whichever started first.

## Requirements

- **FR-001**: A search started by a source change, a filter change, a sort
  change or a sort-direction change MUST show the header progress bar and
  nothing else.
- **FR-002**: A search started by a TYPED query MUST still show the spinner and
  Cancel block.
- **FR-003**: Neither paging nor a pull MUST show the block.
- **FR-004**: What a search shows MUST be decided by what STARTED it, not by
  whether a request happens to be in flight.
- **FR-005**: The progress bar MUST keep its row whether or not a search is
  running, so nothing moves as it comes and goes.

## Success Criteria

- **SC-001**: Through a sort, filter and source change, no Cancel control is on
  screen at any point, sampled across the whole load.
- **SC-002**: Through a typed search, it is.
- **SC-003**: The grid does not move when the block appears.

## Credential-Safety Impact

- **None.** Presentation only.

## Assumptions

- The origin travels with the search rather than being inferred afterwards.
  Inferring it from state (is there a query? did the filters change?) would get
  the last case above wrong, because by the time the question is asked the state
  belongs to whichever search started most recently.
