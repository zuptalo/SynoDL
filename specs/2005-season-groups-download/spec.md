# Feature Specification: Obvious season dividers in the download options list

**Feature Branch**: `fix/2005-season-groups-download`

**Created**: 2026-07-30

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device feedback on spec 2004: the season ordering is right, but every row carries the same
hairline separator, so where one season ends and the next begins is hard to spot in a long pack list.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Season groups are visually separated (Priority: P1)

Scanning a multi-season series' download options, the user can't quickly see where a season's packs
end and the next season's begin — every row looks equally separated. They want an obvious divider at
each season change.

**Why this priority**: Directly follows the ordering fix; without it the ordering's benefit is muted
on long lists.

**Independent Test**: Open a multi-season series in a quality tier; a clearly stronger divider appears
at every season change, and rows within a season keep the normal subtle separator.

**Acceptance Scenarios**:

1. **Given** a series with packs across several seasons, **When** the list renders, **Then** a stronger
   divider marks the first row of each new season.
2. **Given** rows within the same season, **When** they render, **Then** they keep the normal row
   separator (only season changes are emphasised).
3. **Given** the first row of the list, **When** it renders, **Then** it carries no divider above it.
4. **Given** a movie (options have no season), **When** the list renders, **Then** no season dividers
   appear.

### Edge Cases

- A pack with a blank/unparseable season doesn't create a spurious divider.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The download options list MUST draw a visually stronger divider at each season change.
- **FR-002**: Rows within the same season MUST keep the normal row separator.
- **FR-003**: The first row MUST NOT carry a season divider, and movie lists MUST show none.
- **FR-004**: Ordering, selection, and sending behaviour MUST be unchanged (presentation only).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a multi-season list, every season boundary is visually distinguishable from an
  in-season row boundary at a glance.
- **SC-002**: No divider appears above the first row or anywhere in a movie's options.

## Assumptions

- The emphasis is a stronger rule (accent-coloured top border plus a little spacing) rather than
  inserting group-header rows, keeping the list compact — the season is already named on every row.
