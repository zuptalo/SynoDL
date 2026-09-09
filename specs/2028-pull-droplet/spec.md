# Feature Specification: Pulling stretches a droplet, and the cancel rides with the spinner

**Feature Branch**: `fix/2028-pull-droplet`

**Created**: 2026-09-09

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "When pulling down, an animation for something like a droplet 💧 which
is being stretched down until it converts to a spinning spinner just like you
have already, while a cancel button with an ✕ shows underneath it, and they both
scroll up in the same position to each other and remain visible until the result
comes in or the request times out due to network or server-side issues."

## Overview

Spec 1041 gave a running search a cancel, and spec 2027 moved it off the controls
it was covering. What neither did was make the pull gesture and the cancel feel
like one thing.

Today the pull shows Ionic's stock arrow, the refresher shows its own spinner,
and the cancel is a separate horizontal pill floating nearby — so a pull-to-
refresh puts **two** spinners on screen a few pixels apart, each saying the same
thing, with the cancel beside one of them rather than belonging to either.

The described interaction is one object throughout: a droplet drawn out by the
pull, which becomes the spinner, with the cancel directly beneath it, the pair
holding their position relative to each other until the search ends.

## User Scenarios & Testing

### User Story 1 - Pulling to refresh (Priority: P1)

Someone pulls the list down and watches a droplet stretch under their finger,
turn into the spinner, and stay — with a cancel under it — until results arrive.

**Acceptance Scenarios**:

1. **Given** the list is pulled down, **When** the finger moves, **Then** the
   droplet stretches downward rather than merely growing.
2. **Given** the pull passes its threshold, **When** it is released, **Then** the
   droplet gives way to the spinner in the same place.
3. **Given** a search is running, **When** the screen is watched, **Then** there
   is exactly ONE spinner.
4. **Given** the search finishes or fails, **When** it ends, **Then** the spinner
   and the cancel both go.

---

### User Story 2 - The cancel belongs to the spinner (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a search is running, **When** the pair is shown, **Then** the cancel
   is directly beneath the spinner and centred on it.
2. **Given** the list moves, **When** it does, **Then** the two keep their
   position relative to each other.
3. **Given** a search started by a sort, a filter or a keystroke, **When** it
   runs, **Then** it shows the same pair — the treatment does not depend on how
   the search began.
4. **Given** a pull-to-refresh is cancelled, **When** it is, **Then** the
   refresher retracts with it rather than staying open over nothing.

---

### Edge Cases

- **A search that fails or times out.** The refresher must retract on the error
  path too; a refresher left open is the screen saying "working" about nothing.
- **A cancel with no pull behind it.** Cancelling a sort-triggered search has no
  refresher to retract, and must not assume one.

## Requirements

- **FR-001**: The pull indicator MUST be a droplet that stretches DOWNWARD as the
  pull deepens.
- **FR-002**: Exactly one spinner MUST be on screen while a search runs.
- **FR-003**: The cancel MUST sit directly beneath the spinner and centred on it,
  as one block.
- **FR-004**: The pair MUST hold that relationship however the list moves.
- **FR-005**: The pair MUST be the same whatever started the search.
- **FR-006**: The pair MUST remain until the search finishes OR fails.
- **FR-007**: Cancelling a pull-triggered search MUST retract the refresher.
- **FR-008**: The pair MUST still cost no layout.

## Success Criteria

- **SC-001**: The cancel's top edge is below the spinner's bottom edge, and their
  centres line up.
- **SC-002**: A search that errors leaves no refresher open.

## Credential-Safety Impact

- **None.** Presentation only.

## Assumptions

- The droplet's stretch comes from the refresher's OWN pull progress, by giving
  the icon a top transform-origin so the scale it already applies extends the
  shape downward. That ties the animation to the finger rather than running
  alongside it, and needs nothing Ionic does not already expose.
