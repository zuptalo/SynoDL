# Feature Specification: The cancel pill, clear of the controls it covered

**Feature Branch**: `fix/2027-cancel-pill-position`

**Created**: 2026-09-09

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Reported from use with a screenshot: the cancel pill added by spec
1041 sits on top of the search box and the sort control. "Could you please put
the cancel option down below the searching spinner instead?"

## Overview

The pill was positioned with `calc(var(--app-header-h, 108px) + 10px)`.

**`--app-header-h` is defined nowhere in the app.** It was invented at the same
moment it was used, so the expression has only ever evaluated its fallback — and
108px is shorter than this header actually is, which is a title bar, a search box
and a sort control stacked together. The pill therefore landed inside the header
rather than below it, covering the two controls a reader is most likely to want
while a search is running.

The e2e written alongside it did not catch this, and that is worth recording. It
asserted the pill did not move the grid, which is true and was the property under
test. The test viewport's header is short enough that 118px clears it, so the
placement passed there and failed on a phone.

## User Scenarios & Testing

### User Story 1 - Cancelling without losing the controls (Priority: P1)

Someone watching a slow search sees the cancel offer sitting in open space, not
over the search box.

**Independent Test**: start a search and confirm the pill's top edge is below the
header and below where the refresher's spinner appears.

**Acceptance Scenarios**:

1. **Given** a search is running, **When** the pill appears, **Then** it is
   entirely below the header.
2. **Given** a pull-to-refresh is running, **When** the pill appears, **Then** it
   is below the refresher's spinner rather than on top of it.
3. **Given** the header's own height changes — the sort hint appearing, a source
   picker being shown — **When** the pill appears, **Then** it is still below it.
4. **Given** the pill appears or goes, **When** the screen is watched, **Then**
   nothing else moves, exactly as before.

---

### Edge Cases

- **A header that changes height after the pill is placed.** Rotating the device,
  or the sort hint appearing, must not leave the pill stranded over a control.
- **No spinner showing.** An ordinary sort change has no refresher spinner; the
  pill simply sits over the results, which is what a floating control does.

## Requirements

- **FR-001**: The pill MUST be positioned from the header's ACTUAL height, never
  from a guessed one.
- **FR-002**: The pill MUST sit below the refresher's spinner, so two things do
  not say "working" in the same place.
- **FR-003**: The pill MUST still cost no layout: nothing moves as it appears or
  goes.
- **FR-004**: The position MUST be re-derived when the header's height can have
  changed.

## Success Criteria

- **SC-001**: The pill's top edge is below the header's bottom edge, with
  clearance for the spinner.
- **SC-002**: A test fails if the pill is placed by a fixed offset again.

## Credential-Safety Impact

- **None.** A CSS position.

## Assumptions

- Measuring the header on mount, on resize, on orientation change and whenever
  the pill is about to be shown covers every way its height changes in practice.
