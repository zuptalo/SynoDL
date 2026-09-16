# Feature Specification: The last title's cast stops appearing on the next one

**Feature Branch**: `fix/2033-people-section-title-sheet`

**Created**: 2026-09-16

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "when opening a new title, the casts and director section from the old item that we have looked at shows up on the loading page before the current one loads up, also let's move the casts and director section above the download link section and let's also drop the icon which shows they will open up in a new tab"

## Overview

Spec 0014 shipped the people behind a title an hour ago. Opening a second title
shows the FIRST one's cast, under the spinner, until the new one arrives — so for
a moment the sheet confidently attributes one film's actors to another.

Two things cause it together, and both are worth fixing rather than either alone:

1. **The sheet never forgets.** Opening a title resets the download options, the
   ownership marker, the poster state and the catalog entry — but not the
   metadata the title's own detail response supplied. That miss predates spec
   0014: the synopsis, IMDb id and year have been carried over the same way since
   spec 1023. It was invisible because each of those is only shown when the
   catalog row has none, so the stale value lost a race it usually lost anyway.
   A cast has no such competitor, so it simply shows.
2. **The people are drawn outside the loading state.** The section sits after the
   block the spinner replaces, so it renders *during* the spinner rather than
   instead of it. Even correctly-cleared data would flash empty here; stale data
   flashes wrong.

Two adjustments ride along, both reversing calls made in 0014 now that the thing
exists and can be looked at:

- **The people move above the download options.** 0014 put them below on the
  reasoning that sending a download is the sheet's job. Seeing it, the ordering
  reads backwards: the cast is part of deciding *whether* you want the thing,
  which happens before choosing which file of it to fetch.
- **The new-tab icon goes.** Every tile carries one, so it decorates rather than
  distinguishes, and it crowds a 76-pixel tile whose name already wraps.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The sheet never shows me the wrong people (Priority: P1)

Opening one title and then another shows the second title's cast, or nothing at
all while it loads — never the first title's.

**Why this priority**: It is the bug. Showing a real cast under a different
film's name is worse than showing no cast, because there is nothing about it that
looks like a loading state.

**Independent Test**: Open a title with a cast, close it, open a different one,
and confirm that at no point during the load are the first title's people on
screen.

**Acceptance Scenarios**:

1. **Given** a title whose people have been shown, **When** a different title is
   opened, **Then** the previous title's people are never displayed.
2. **Given** a title is still loading, **When** the spinner is shown, **Then** no
   people section is shown at all.
3. **Given** the second title's source publishes nobody, **When** it finishes
   loading, **Then** no people section appears — not the previous title's.
4. **Given** a title is reopened from the Tasks list rather than the grid,
   **When** it loads, **Then** the same rules hold on that path too.
5. **Given** the synopsis, IMDb link and year that arrive by the same route,
   **When** a new title is opened, **Then** none of them carries over either.

---

### User Story 2 - The cast reads before the download options (Priority: P2)

The people sit between the synopsis and the download options: what the thing is,
who is in it, then which file to fetch.

**Why this priority**: An ordering improvement to something that already works,
not a defect.

**Independent Test**: Open a title with both and confirm the people section is
above the options.

**Acceptance Scenarios**:

1. **Given** a title with people and download options, **When** the sheet is
   open, **Then** the people are above the options.
2. **Given** a title opened read-only from a task, **When** the sheet is open,
   **Then** the people still appear, in the same place relative to the synopsis.

---

### User Story 3 - A tile is not crowded by an icon (Priority: P3)

A person's tile shows their face, their name and their character. Nothing else.

**Why this priority**: Cosmetic.

**Acceptance Scenarios**:

1. **Given** a person whose tile links to IMDb, **When** the tile renders,
   **Then** no external-link icon is shown.
2. **Given** that tile, **When** it renders, **Then** it is still discoverable as
   a link to assistive technology and still opens externally.

---

### Edge Cases

- **Reopening the SAME title.** Clearing on open must not cause a visible flicker
  of a section that is about to be repopulated identically — the clear happens
  behind the spinner.
- **A title opened read-only from a task**, which takes a different load path and
  must clear the same state.
- **A source that fails to answer.** The section stays absent rather than falling
  back to whatever was there before.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Opening a title MUST clear every piece of metadata carried by the
  previous title's detail response — its people, synopsis, IMDb id and year —
  before the new one is requested.
- **FR-002**: The people section MUST NOT be rendered while the sheet is
  loading; the spinner is the whole of the loading state.
- **FR-003**: FR-001 and FR-002 MUST hold on both paths that open the sheet: from
  the Discover grid, and read-only from a task.
- **FR-004**: The people section MUST sit between the synopsis and the download
  options.
- **FR-005**: A person's tile MUST NOT show an external-link icon, and MUST
  remain a link that opens externally and is announced as one.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across any sequence of titles opened in any order, no title's
  people are ever shown under another title's name.
- **SC-002**: A title that is still loading shows a spinner and nothing else.
- **SC-003**: The people are read before the download options on every title that
  has both.

## Assumptions

- **Nothing about the data changes.** This is entirely about when the sheet shows
  what it already has; no driver, endpoint, cache or stored value is touched.
- **The reordering is a reversal, not a discovery.** Spec 0014 chose below
  deliberately and this chooses above deliberately; the earlier reasoning is
  recorded there and does not need re-arguing.
