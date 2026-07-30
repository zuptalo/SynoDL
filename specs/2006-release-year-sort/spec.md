# Feature Specification: Release-year sort no longer leads with year-less titles

**Feature Branch**: `fix/2006-release-year-sort`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device feedback: sorting Discover by "Release year" shows a handful of apparently random
titles at the top, and reversing the direction makes the whole first stretch look unsorted.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Release-year sort reads as sorted from the first row (Priority: P1)

Browsing Discover sorted by release year, the user expects the very first results to be the newest
(descending) or the oldest (ascending). Today the first rows are titles the source has no usable
release year for, so the list opens on what looks like random content — and ascending stays that way
for the first several screens of scrolling.

**Why this priority**: It is the first thing seen after choosing the sort; it makes a working sort
look broken.

**Independent Test**: Choose "Release year" in either direction and look at the first screen — every
visible title belongs at that end of the range, and scrolling stays monotonic.

**Acceptance Scenarios**:

1. **Given** Discover sorted by release year descending, **When** the first page loads, **Then** the
   leading titles are the newest ones, with no year-less or implausible-year titles ahead of them.
2. **Given** Discover sorted by release year ascending, **When** the first pages load, **Then** the
   leading titles are the oldest ones and the years increase as the user scrolls.
3. **Given** the user has set their own "Year from" / "Year to" filter, **When** results load,
   **Then** their bounds are used unchanged.
4. **Given** any other sort (most popular, IMDb rating, date added), **When** results load, **Then**
   the result set is unchanged from before this fix.

### Edge Cases

- Only one of the two year filters set: the user's bound is kept and only the missing side is
  supplied.
- Upcoming titles dated beyond the source's own year facet (2027/2028) MUST still appear.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When browsing sorted by release year, the request MUST carry a plausible release-year
  range so the source's year-less and garbage-year rows are excluded.
- **FR-002**: A year bound the user set in the filter sheet MUST take precedence; only an unset bound
  is supplied by default.
- **FR-003**: The implicit range MUST be wide enough to keep genuine upcoming titles (beyond the
  source's advertised year facet maximum).
- **FR-004**: Sorts other than release year MUST send exactly the parameters they send today.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With release-year descending, the first page contains only titles from the newest years
  the catalog holds.
- **SC-002**: With release-year ascending, the first page contains only the oldest titles in the
  catalog (1890s–1900s), and no year-less titles.
- **SC-003**: Switching direction produces a list that is the mirror of the other, not a different
  ordering scheme.

## Assumptions

- Verified live against the source API (2026-07-30): `orderby/year` sorts correctly in both
  directions at depth; the disorder is confined to ~350 rows whose year column is empty or
  implausible. Empty ones sort first ascending (~13 pages) and last descending; implausible ones
  (e.g. a title carrying "7441") sort above the newest titles descending. Sending `min_year` /
  `max_year` removes exactly those rows — the catalog goes from 4413 to 4400 pages, ascending then
  opens at 1894 and descending at 2028.
- The source's own year facet reports 1890–2026 while real titles are dated up to 2028, so the
  implicit upper bound is deliberately wider than that facet.
- A post's JSON carries no year field at all — the year lives only in the title text — so this is
  fixed by bounding the query, not by re-sorting results in the app.
