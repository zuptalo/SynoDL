# Feature Specification: Release-year sort is fast again, and Discover opens on Most popular

**Feature Branch**: `fix/2007-release-year-sort`

**Created**: 2026-07-30

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Timing spec 2006 on the live instance showed its implicit year bounds make the source's
database an order of magnitude slower on any page it hasn't cached. Reverting is the right trade;
Most popular becomes the default sort everywhere.

## Overview

Spec 2006 excluded the source's ~350 broken-year rows by sending `min_year` / `max_year` with the
release-year sort. The ordering it produced was correct, but measurement afterwards (same page,
with and without the bounds, straight at the source) showed the filter is what makes their query
slow:

| `year` sort, fresh page | ascending | descending |
|---|---|---|
| no bounds | 1.5 s | 1.9 s |
| `min_year` only | 18.6 s | 20.4 s |
| `max_year` only | — | 4.8 s |
| both (spec 2006) | 17.8 s | 16.7 s |

`min_year` is both the bound that removes the broken rows and the expensive one; a low value
(`min_year=1`) is no cheaper (13–21 s), so the cost is the filter itself, not the value. Other
sorts on the same pages run 0.4–3.9 s. With spec 1018 pulling two pages per scroll trigger, the
regression lands twice per trigger.

What comes back on revert is small: the source's broken rows lead the descending list, but 8 of the
12 leading rows are coming-soon titles the app already hides, leaving **4 odd rows** at the top.
Ascending's opening stretch is oddly ordered again — about a dozen visible titles, the rest of that
block being coming-soon and hidden.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browsing by release year doesn't stall (Priority: P1)

Scrolling Discover sorted by release year hits pages the source hasn't cached, and each one takes
15–20 seconds — the grid stops dead. The user would rather see a few oddly-placed titles at the top
than wait.

**Why this priority**: A multi-second stall mid-scroll is a far worse experience than a cosmetic
ordering flaw, and it affects every uncached page rather than just the first screen.

**Independent Test**: Sort by release year and scroll into fresh pages; each page arrives in about
the same time as the other sorts.

**Acceptance Scenarios**:

1. **Given** the release-year sort, **When** a page is requested, **Then** no implicit year bounds
   are sent and the request completes in the same range as the other sorts.
2. **Given** the user set their own "Year from" / "Year to", **When** results load, **Then** those
   bounds are sent exactly as the user set them.
3. **Given** any other sort, **When** results load, **Then** nothing about the request changes.

### User Story 2 - Discover opens on Most popular (Priority: P2)

The release-year sort leads with the source's broken rows, so it is the wrong thing to land on.
Most popular is the sort that reads best on arrival.

**Why this priority**: Follows directly from US1 — reverting means release year is no longer a good
default landing view.

**Independent Test**: A client that sends no sort gets Most popular, matching what the app already
defaults to.

**Acceptance Scenarios**:

1. **Given** a request with no sort specified, **When** results load, **Then** they are ordered by
   Most popular, descending.
2. **Given** an unrecognised sort value, **When** results load, **Then** it falls back to Most
   popular rather than release year.
3. **Given** a user who has chosen a sort, **When** they return to Discover, **Then** their saved
   choice still applies — the default only fills the gap.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The release-year sort MUST NOT send implicit year bounds.
- **FR-002**: Year bounds the user set in the filter sheet MUST still be sent unchanged, under any
  sort.
- **FR-003**: An empty or unrecognised sort MUST resolve to Most popular, descending — matching the
  client's own default so the two cannot drift.
- **FR-004**: A user's saved sort choice MUST continue to take precedence over the default.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A fresh page under the release-year sort loads in the same range as the other sorts
  (single-digit seconds), rather than 15–20 s.
- **SC-002**: Opening Discover with no saved view lands on Most popular.

## Assumptions

- The 4 odd rows at the head of the descending release-year list are accepted as a source data
  defect we cannot fix cheaply. Detecting them per-page is possible for descending but not for
  ascending (that block spans ~13 pages), so no half-fix is attempted here.
- Spec 2006 is superseded by this one; its behaviour is removed rather than amended.
