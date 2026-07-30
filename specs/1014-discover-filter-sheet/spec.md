# Feature Specification: Discover filter sheet polish

**Feature Branch**: `feat/1014-discover-filter-sheet`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User testing feedback on spec 2002 (the search honesty fix): the hint wording and
placement, marking of ineffective controls, two mismatched dropdowns, and filter-sheet styling.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ineffective sort/filters are clearly marked during search (Priority: P1)

While searching, the user still sees their previously-chosen sort and filter chips (Sci-Fi, 4K,
≥2000, IMDb-rating sort). Today they look normal even though only the Type filter actually applies.
The user wants the ones that don't apply to be visibly struck through / marked as ineffective, so it
is obvious at a glance which of their choices are doing nothing right now.

**Why this priority**: This is the core clarity win — it turns "why isn't my filter working" into an
at-a-glance answer, on the chips the user is already looking at.

**Independent Test**: Search a term with a genre/quality/year/sort active; confirm every chip except
the Type chip, and the sort label, render struck-through/dimmed; clear the search and confirm they
return to normal.

**Acceptance Scenarios**:

1. **Given** a search is active with non-type filters and a non-default sort, **When** the results
   show, **Then** the non-type filter chips and the sort label are struck through / marked
   ineffective, while the Type chip stays normal.
2. **Given** the search is cleared, **When** browsing resumes, **Then** all chips and the sort label
   render normally again.

---

### User Story 2 - The search hint reads cleanly and scrolls away (Priority: P2)

The hint under the search bar stays pinned at all times and uses an awkward dash. The user wants it
to (a) read cleanly without the dash, (b) only nudge them to "clear the search" when they actually
have ineffective filters/sort selected, and (c) scroll with the results (sitting with the chips at
the top of the list) rather than stay stuck under the search bar.

**Why this priority**: A refinement of the same message — lower stakes than the at-a-glance marking,
but it removes visual noise from a permanently-pinned banner.

**Acceptance Scenarios**:

1. **Given** a search is active, **When** the user reads the hint, **Then** it contains no dash and
   the "clear the search to sort or filter" nudge appears only when an ineffective filter or sort is
   actually selected.
2. **Given** a search is active, **When** the user scrolls the results, **Then** the hint scrolls
   away with the chips instead of remaining pinned under the search bar.

---

### User Story 3 - The filter sheet looks and sections like Settings (Priority: P3)

The filter sheet's Type and Min-rating controls open an awkward in-place dropdown while every other
filter opens a centered dialog, and the sheet has one unlabeled group plus a single "More filters"
header. The user wants Type and Min rating to match the others, and the whole sheet to be sectioned
with clear headers like the Settings screen.

**Acceptance Scenarios**:

1. **Given** the filter sheet is open, **When** the user taps Type or Min rating, **Then** it opens
   the same centered dialog the other filters use (not an in-place, edge-anchored dropdown).
2. **Given** the filter sheet is open, **When** the user scans it, **Then** its controls are grouped
   into clearly-titled sections styled like the Settings screen.

---

### Edge Cases

- Search active but no ineffective filter/sort selected: the chips row may only contain the Type
  chip (normal), the hint shows its base sentence without the "clear the search" nudge.
- No filters at all + search active: the hint still explains that only the type filter narrows
  results; nothing is struck through because nothing is selected.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: While a search is active, every active filter chip except Type, and the sort label,
  MUST be visually marked as ineffective (struck through and dimmed).
- **FR-002**: Clearing the search MUST return all chips and the sort label to their normal styling.
- **FR-003**: The search hint MUST NOT contain a dash and MUST read as plain sentences.
- **FR-004**: The "clear the search to sort or filter" nudge MUST appear only when at least one
  ineffective filter or a non-default sort/order is actually selected.
- **FR-005**: The search hint MUST live in the scrolling results area (with the active-filter chips),
  not pinned under the search bar.
- **FR-006**: The Type and Min-rating controls MUST open the same centered dialog the other filter
  controls use, not an in-place edge-anchored dropdown.
- **FR-007**: The filter sheet MUST be organised into clearly-titled sections styled consistently
  with the Settings screen.
- **FR-008**: Browse and search behaviour (which results come back, and that only Type narrows
  search) MUST be unchanged — this is presentation only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: During a search, 100% of ineffective selections (non-type chips + sort) read as struck
  through; the Type chip and any effective control do not.
- **SC-002**: The hint contains zero dash characters and is absent from the fixed header (it scrolls).
- **SC-003**: Type and Min rating use the identical dialog style as the other five facet selects.

## Assumptions

- "Ineffective" during search = every facet filter except Type, plus the sort field/direction (the
  provider ranks text results by relevance only — verified in spec 2002).
- Marking = strikethrough + reduced opacity (consistent, accessible, theme-aware).
- Settings-like styling = `ion-list inset` groups each led by an `ion-list-header` title.
