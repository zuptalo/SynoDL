# Feature Specification: Order series download options by season then size

**Feature Branch**: `fix/2004-series-download-options`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device feedback: in a title's download-options list, a series' season packs come out
jumbled (S6, S7, S1, S1, S2, S2, S3 …) because they're sorted by file size only. They should read in
season order.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Season-ordered download options (Priority: P1)

Opening a series in Discover and picking a quality tier shows the season packs sorted by size, so
seasons are interleaved and hard to scan. The user wants them grouped by season (ascending), and
within a season the largest file first.

**Why this priority**: The list is a core part of choosing what to download; the current order makes
finding a season needlessly hard.

**Independent Test**: Open a multi-season series, pick a quality tier; the packs read Season 1, 2,
3 … in order, and multiple packs of the same season are largest-first.

**Acceptance Scenarios**:

1. **Given** a series with packs across several seasons in a quality tier, **When** the list renders,
   **Then** packs are ordered by season ascending, and within a season by size descending.
2. **Given** a movie (options have no season), **When** the list renders, **Then** the options are
   ordered largest file first (unchanged).
3. **Given** the default pre-selection, **When** the modal opens, **Then** the pre-selected option is
   the first usable option in that display order (so the checkmark isn't stranded mid-list).

### Edge Cases

- A pack with an unparseable/blank season sorts as season 0 (with the movies), not mid-list.
- Two packs of the same season and same size keep a stable relative order.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Series download options MUST be ordered by season number ascending, then by file size
  descending within a season.
- **FR-002**: Movie options (no season) MUST remain ordered by file size descending.
- **FR-003**: The default pre-selected option MUST be the first usable option in the same display
  order (consistent with the list), preserving the size-cap "usable" rule.
- **FR-004**: Client-only, presentation/selection ordering — no server, API, or data change.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any multi-season series, no season appears out of ascending order in a tier's list.
- **SC-002**: Within a season, packs are largest-first; movies are largest-first as before.

## Assumptions

- The season number is parsed from the pack's season label ("Season 6" → 6), which is how the option
  already carries it; unlabeled packs sort as 0.
