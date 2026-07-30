# Feature Specification: Download statistics — readable history + totals

**Feature Branch**: `feat/1017-download-statistics-readable`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device-testing feedback on the Download statistics modal: the history chart renders as one
big solid rectangle and is hard to understand, and there's no total shown for the selected user/range.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A readable history chart (Priority: P1)

The history chart currently shows one solid green rectangle filling the box — no scale, no readable
bars, nothing to interpret. The user wants a chart they can actually read: distinct bars per period,
values legible, and it must not stretch into a single block.

**Why this priority**: The chart is the centrepiece of the History section and today conveys nothing.

**Independent Test**: Open statistics with some history; the chart shows separate bars per period with
their values readable and a sense of scale; a single period shows one normal-width labelled bar, never
a full-box rectangle.

**Acceptance Scenarios**:

1. **Given** history spanning multiple periods, **When** the chart renders, **Then** each period is a
   distinct bar sized to its value, with the period labels and a way to read each bar's count.
2. **Given** history with only one period of activity, **When** the chart renders, **Then** it shows a
   single normal-width, labelled bar — not a rectangle stretched to fill the plot.
3. **Given** no history, **When** the chart renders, **Then** an empty-state message is shown.

---

### User Story 2 - "All" shows a real overview, not one bar (Priority: P1)

Selecting the "All" range collapses everything into a single "All time" bar, which is meaningless. The
user wants "All" to show the whole history at a sensible granularity so it reads as a trend.

**Why this priority**: "All" is the default landing for a full picture and is the worst offender for
the rectangle.

**Acceptance Scenarios**:

1. **Given** history spanning multiple years, **When** "All" is selected, **Then** the chart shows one
   bar per year.
2. **Given** history within a single year but multiple months, **When** "All" is selected, **Then** the
   chart shows one bar per month.
3. **Given** history within a single month, **When** "All" is selected, **Then** the chart shows one
   bar per day.

---

### User Story 3 - A total for the selected user and range (Priority: P2)

There's no headline total. The user wants to see the total downloads (and total size) for the current
selection (user + source), shown prominently near the history.

**Acceptance Scenarios**:

1. **Given** a user/source selection, **When** the statistics show, **Then** a total downloads count
   and total size for that selection is displayed prominently.
2. **Given** the selection changes (user or source), **When** it updates, **Then** the total updates
   to match.

---

### Edge Cases

- A single data point must not stretch to fill the chart (bounded bar width).
- The total's size is "—" when nothing has completed (no bytes yet), not "0 B" implying a real value.
- Many periods (e.g. daily over a long span) must stay legible — labels thin out and the plot scrolls
  rather than crushing bars to slivers.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The history chart MUST render distinct, readable bars per period (no full-box rectangle),
  correct in light and dark themes.
- **FR-002**: Each bar's value MUST be legible (a direct value label when the bar count is few, and a
  hover/press tooltip otherwise), with a visible sense of scale (baseline + max reference).
- **FR-003**: A single-period series MUST render one bounded-width labelled bar, not a stretched block.
- **FR-004**: The "All" range MUST bucket the full history adaptively — by year when it spans multiple
  years, by month within a single year spanning multiple months, else by day.
- **FR-005**: The statistics MUST show a prominent total (download count and total size) for the
  currently selected user and source, updating when the selection changes.
- **FR-006**: An empty history MUST show an empty-state message, not a broken/blank chart.
- **FR-007**: No server or data-model change is required; bucketing and totals are derived client-side
  from the existing summary and timeseries (stateless-proxy boundary untouched).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The history chart never renders as a single full-area rectangle for any range.
- **SC-002**: For a multi-period selection, each bar's value is readable (label or tooltip) and the
  chart communicates relative magnitude at a glance.
- **SC-003**: A total (count + size) for the selected user/source is visible without scrolling to the
  per-type list, and matches the per-type Overall.

## Assumptions

- The history "range" segments are granularities over the full data (not time windows), so the total
  reflects the user/source selection; "All" is made meaningful by adaptive bucketing rather than a new
  windowed query.
- Timeseries carries counts only (no per-day bytes), so the range chart is by count; the total's size
  comes from the summary's byte totals for the selection (matching the per-type Overall).
- A single accent-green series needs no legend (the section title names it); value labels/tooltips keep
  identity from being colour-alone.
