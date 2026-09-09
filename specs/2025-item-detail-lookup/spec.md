# Feature Specification: An item's details, actually reachable

**Feature Branch**: `fix/2025-item-detail-lookup`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Reported from use: tapping any track inside a playlist opened a sheet
reading "This download is no longer available", whatever state the track was in.

## Overview

The detail sheet resolved which download to show by looking it up in the Tasks
list. A group's ITEMS are deliberately absent from that list — spec 0013 FR-019b
keeps them behind the group row so an uncapped channel cannot crowd out
everything else — so an item was never found, and the sheet fell straight through
to its "no longer available" state.

That state is meant for a download dismissed while its sheet was open. It was
being shown for every item, always.

`GET /v1/ytdl/{requestId}` was built for exactly this in spec 0013 and never
wired to a client method, so the sheet had no way to ask for a row the list does
not carry.

## User Scenarios & Testing

### User Story 1 - Opening a track inside a playlist (Priority: P1)

Someone opens a playlist, taps one of its tracks, and sees that track's details.

**Independent Test**: expand a playlist, tap an item, and confirm the sheet shows
its title, link and origin rather than a gone message.

**Acceptance Scenarios**:

1. **Given** an expanded playlist, **When** a track is tapped, **Then** its
   details are shown.
2. **Given** that sheet is open on a running track, **When** its progress moves,
   **Then** the sheet follows it.
3. **Given** a download that really has been dismissed, **When** its sheet is
   open, **Then** it says so.

---

### Edge Cases

- **A lookup still in flight.** Nothing is known yet, and that is not the same as
  the download being gone; the sheet must not accuse before it has asked.
- **A row the list does carry.** A single download or a group still comes from
  the list, which is already live — no second request for something already at
  hand.

## Requirements

### Functional Requirements

- **FR-001**: The detail sheet MUST be able to show any download the caller may
  see, including one absent from the Tasks list.
- **FR-002**: A row present in the list MUST be taken from the list, so it stays
  live without extra requests.
- **FR-003**: The "no longer available" state MUST be shown only after a lookup
  has concluded that the download is gone.
- **FR-004**: A fetched row MUST keep updating while its sheet is open, and MUST
  stop when it closes.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Tapping any item of an expanded playlist shows its details.
- **SC-002**: A regression test fails if the sheet can no longer fetch a row the
  list does not carry.

## Assumptions

- The endpoint's ownership rule is unchanged: a download the caller may not see
  answers 404 and therefore reads as unavailable, which is correct.
