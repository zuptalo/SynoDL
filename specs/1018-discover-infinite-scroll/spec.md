# Feature Specification: Discover keeps loading ahead of a fast scroller

**Feature Branch**: `feat/1018-discover-infinite-scroll`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device feedback: scrolling Discover quickly keeps stopping at the loading spinner —
the grid only ever loads one page at a time, so a fast scroller is always waiting for it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fast scrolling stays smooth (Priority: P1)

Someone flicking through Discover reaches the end of the loaded results faster than the next page
arrives, so they watch the spinner at the bottom of every page. They want the grid to stay ahead of
them.

**Why this priority**: It's the core interaction of the tab — browsing is the whole point of
Discover, and the stutter is on the happy path.

**Independent Test**: Flick-scroll Discover continuously; results keep appearing without the
spinner becoming the thing you're looking at.

**Acceptance Scenarios**:

1. **Given** a result list with more pages available, **When** the scroll trigger fires, **Then**
   the next two pages are loaded rather than one.
2. **Given** results are loading, **When** the trigger fires again, **Then** no duplicate request is
   made for a page already in flight.
3. **Given** only one more page exists, **When** the trigger fires, **Then** exactly that page is
   loaded and no request is made past the end of the list.
4. **Given** the source starts failing partway through a trigger, **When** the first page errors,
   **Then** the second page is not requested.
5. **Given** the user changes a filter, sort, or search term, **When** results reload, **Then** the
   list resets to the first page as before.

### Edge Cases

- A short result set that doesn't fill the viewport still fills it (the existing viewport-fill
  behaviour must keep working, just in fewer rounds).
- Leaving the tab mid-load must not leave a request chain running for a view the user left.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: One scroll trigger MUST load two pages of results instead of one.
- **FR-002**: Each page MUST appear as it arrives, so results fill in progressively rather than
  waiting for both.
- **FR-003**: The load-ahead MUST stop immediately if the source becomes unavailable, needs
  refreshing, or errors partway through — no further request in that trigger.
- **FR-004**: The load-ahead MUST NOT request pages past the end of the result list.
- **FR-005**: The scroll trigger MUST fire earlier — a full viewport before the end rather than
  just over half of one.
- **FR-006**: Total requests for a given amount of scrolling MUST NOT increase; the same pages are
  fetched, just sooner and in pairs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Continuous fast scrolling through several pages of Discover does not stop at the
  loading spinner.
- **SC-002**: Reaching the end of the results shows the end of the list, with no failed or wasted
  request beyond the last page.
- **SC-003**: A failing source stops the load-ahead within one page instead of firing a second
  request at it.

## Assumptions

- The provider is polite-friendly at this rate: a trigger now peaks at two requests instead of one,
  which is within what the existing debounce and superseded-search guards already produce while
  filtering. No new throttling is needed.
- Discover is a stateful-mode-only feature, so this cannot be covered end-to-end by the current
  e2e harness (it boots the server stateless). Coverage is unit tests over the pagination logic
  plus a manual device pass.
