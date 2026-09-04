# Feature Specification: Statistics filter segments sit in cards

**Feature Branch**: `fix/2003-statistics-filter-segments`

**Created**: 2026-07-30

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device feedback: the Download-statistics segments (source, and Day/Week/Month/Year/All)
float full-width between the cards instead of sitting inside a card like the Settings screen's
segments — Ionic handles the carded look fine, so this is a layout bug in our statistics view.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Carded filter controls (Priority: P1)

The statistics modal reuses the grouped-card look, but its source segment and the history-range
segment are bare children of the view, so they render full-bleed between the rounded cards and break
the rhythm. The user wants them contained in cards like the Settings screen's "Open to" segment.

**Why this priority**: A visible layout inconsistency on a primary screen; the fix is small and
matches an existing, correct pattern.

**Acceptance Scenarios**:

1. **Given** the statistics modal is open, **When** the source filter renders, **Then** it sits inside
   a rounded card (grouped with the user picker for admins), not floating between cards.
2. **Given** the History section, **When** the range segment renders, **Then** it sits inside the
   History card, aligned with the card's padding, matching the Settings "Open to" segment.
3. **Given** the Settings screen and the statistics modal side by side, **When** compared, **Then**
   their segment/card treatment is visually consistent.

### Edge Cases

- A non-admin (no user picker) still gets a properly-carded source segment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The statistics source segment MUST render inside an inset card (grouped with the user
  picker when present), not full-bleed between cards.
- **FR-002**: The history-range segment MUST render inside the History card, padded/aligned like the
  Settings screen's in-card segment.
- **FR-003**: Behaviour (filtering, bucket switching, totals, chart) MUST be unchanged — layout only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: No statistics segment renders outside a card; the modal's card rhythm matches Settings.
- **SC-002**: Source/user filtering and range switching behave exactly as before.

## Assumptions

- The intended look is the existing `settings-cards` + in-item segment pattern used by the Settings
  screen (`ion-item lines="none"` wrapping an `ion-segment`), so this mirrors it rather than inventing
  new styling.
