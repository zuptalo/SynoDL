# Feature Specification: A group's items stop reloading on every poll

**Feature Branch**: `fix/2024-group-sheet-refetch`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Reported from use: "the sub tasks under a playlist [get] refreshed
every 5 seconds or so".

## Overview

The list of a group's items is fetched when the sheet opens. It was also being
cleared and refetched every few seconds, which reads as a flicker and throws away
any further pages the reader has scrolled to.

The cause is a Vue watcher source. It was written as one getter returning an
array:

```ts
watch(() => [props.isOpen, props.group?.requestId], …)
```

That getter builds a NEW array every evaluation, and a non-deep watcher compares
by reference, so it fired on every re-render — and `group` comes from a list the
app re-reads every few seconds. The rest of the codebase writes multi-source
watches as an array of sources, which Vue compares element-wise.

The same trap sits one layer down: `group.counts` is a new object on every poll
even when no number in it has changed, so watching the object — deep or not —
fires just as often.

This is a rendering fault only. Nothing about what is downloaded, stored, or sent
to a worker changes.

## User Scenarios & Testing

### User Story 1 - An open group sheet sits still (Priority: P1)

Someone opens a playlist to watch its items and the list stays put, updating only
when something actually moves.

**Independent Test**: open the sheet on a settled group and count requests to the
items endpoint over several poll intervals.

**Acceptance Scenarios**:

1. **Given** an open group whose items are not changing, **When** several poll
   intervals pass, **Then** the items are not refetched.
2. **Given** an open group, **When** an item's state changes, **Then** the row
   updates without the list being cleared.
3. **Given** a reader who has scrolled to a later page, **When** an update
   arrives, **Then** the loaded pages remain loaded.

---

### Edge Cases

- **A watcher source that is rebuilt each run.** An array or object built inside
  a getter is a new reference every evaluation, so it fires on render rather than
  on change.

## Requirements

### Functional Requirements

- **FR-001**: An open group sheet MUST NOT refetch its items while nothing about
  the group has changed.
- **FR-002**: An update MUST be applied to the rows already shown rather than
  replacing the list.
- **FR-003**: Watcher sources MUST compare by value — separate sources, or a
  derived primitive — never a container rebuilt on each evaluation.

## Success Criteria

### Measurable Outcomes

- **SC-001**: With an open sheet and a settled group, the items endpoint is not
  called again across several poll intervals.
- **SC-002**: A regression test fails if either watcher reverts to a
  rebuilt-container source.

## Assumptions

- The list remains poll-based. Moving these updates onto the existing SSE stream
  is a separate, larger change; this spec removes the wasted work rather than
  changing the transport.
