# Feature Specification: A group's enumeration job is not the group finishing

**Feature Branch**: `fix/2022-expand-job-identity`

**Created**: 2026-09-09

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Observed on the live deployment: a submitted playlist showed its title
and artwork, went straight to "saved", and downloaded nothing. The database held
one group in state `completed` with zero items.

## Overview

A group's enumeration job carries the GROUP's request id. It has to — that is how
the reconciler finds the enumeration belonging to a given group.

But the live-job map that dresses a stored record in its current state is keyed by
request id, so `live[group]` resolved to that enumeration job. The moment the
enumeration succeeded, the group was reported as **completed** and the outcome
written down. The group then left `resolving`, so expansion never ran and the
playlist produced nothing.

Nothing failed. The row looked like a finished download.

The client polls the list every few seconds, so it won that race essentially
every time — the reconciler's own path was already guarded against this, but the
list and detail handlers were not, and they run far more often.

## User Scenarios & Testing

### User Story 1 - A playlist actually expands (Priority: P1)

Someone submits a playlist and it becomes the downloads it contains, rather than
reporting itself finished with nothing to show.

**Why this priority**: it is the whole of spec 0013's US6, and it has not worked
in any shipped build.

**Independent Test**: with a group resolving and its enumeration job succeeded,
read the list, then run a reconcile cycle; the group must expand into its items.

**Acceptance Scenarios**:

1. **Given** a group whose enumeration job has succeeded, **When** the list is
   read before the next reconcile, **Then** the group is NOT reported as
   completed.
2. **Given** the same group, **When** the next cycle runs, **Then** it expands
   into one download per entry.

---

### Edge Cases

- **A running enumeration.** Its output is entries, not progress; reading it as a
  download's progress would attribute a group's enumeration to a download that
  does not exist.
- **Any future job kind sharing a request id.** The collision is structural: a
  request id identifies a REQUEST, and more than one job can serve one request.

## Requirements

### Functional Requirements

- **FR-001**: A group's state MUST be derived from its items, never from any job.
- **FR-002**: An enumeration job MUST NOT be treated as the worker of the
  download whose request id it carries, on any code path.
- **FR-003**: An enumeration job's output MUST NOT be read as download progress.
- **FR-004**: A group whose enumeration has succeeded MUST expand into its items
  regardless of how often the list is read in the meantime.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A submitted playlist expands into one download per entry, with the
  list being polled throughout.
- **SC-002**: No group reaches a final state while it has zero items.

## Credential-Safety Impact

- No change to what is stored, logged, or sent to a worker; no new permission.
  This narrows what a job may be interpreted as, and nothing else.

## Assumptions

- Groups already in a wrong `completed` state from an affected build are not
  migrated: they hold no items and no files were written, so dismissing and
  re-adding is both correct and cheaper than a migration that would have to guess
  at intent.
