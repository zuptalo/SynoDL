# Feature Specification: Discover polish batch

**Feature Branch**: `feat/1015-discover-polish-batch`

**Created**: 2026-07-30

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: A running batch of small Discover refinements from device testing. Each item is a
self-contained tweak; they ship together in one PR/release to avoid a release per micro-change.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Drop the live-streamable filter (Priority: P1)

The filter sheet offers a "Streamable only" toggle that narrows results to titles with a live/stream
copy. SynoDL is a download client and will not offer live streaming, so this filter can only mislead
— a user could exclude perfectly downloadable titles for a capability the app doesn't have. Remove it
end to end (the toggle, its active-filter chip, and the plumbing that carried it to the source).

**Why this priority**: It removes a dead, potentially-confusing control; small and self-contained.

**Independent Test**: Open the filter sheet — there is no "Streamable only" toggle; browsing/searching
behave exactly as before; no request ever carries a `stream` parameter.

**Acceptance Scenarios**:

1. **Given** the filter sheet is open, **When** the user scans the Options section, **Then** there is
   no "Streamable only" toggle (only x265/HEVC and 3D remain).
2. **Given** any browse or search, **When** the request is built, **Then** it carries no `stream`
   filter and results are unchanged from today (minus the never-useful streamable narrowing).

---

<!-- Further batch items (US2, US3, …) are appended here as they arrive, then shipped together. -->

### Edge Cases

- A previously-saved view that still holds `stream:"true"` MUST be ignored gracefully (the field no
  longer exists on the client; it is simply dropped, never sent).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The "Streamable only" filter MUST be removed from the filter sheet UI.
- **FR-002**: The streamable active-filter chip MUST be removed from the results header.
- **FR-003**: No request to the source MUST carry a `stream` filter parameter.
- **FR-004**: The `stream` filter field MUST be removed from the client filter model and the
  server-side request/filter plumbing so no dead code remains.
- **FR-005**: All other browse/search filtering and sorting MUST be unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero occurrences of a streamable filter control or `stream` filter field remain in the
  Discover client and the source request plumbing.
- **SC-002**: Existing browse/search behaviour is otherwise unchanged (all prior source tests pass,
  minus the removed `stream` assertion).

## Assumptions

- SynoDL will not add live streaming, so the filter has no future use — removal, not hiding.
- The source's `stream` parameter support is unused elsewhere; dropping our plumbing for it is safe
  and stays within the existing allowlist (no API surface change).
