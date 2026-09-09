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

## Revision — the animation, redesigned (2026-09-10)

The first cut of this spec shipped as 0.16.14 and reached only the Discover tab:
it gave Ionic's stock pull icon a droplet shape and a top transform-origin, and
put the cancel in a block beneath the spinner.

Watching it run, two things were wrong with the idea rather than the code. The
droplet borrowed Ionic's scale, so it *grew* — it never had a narrow stem being
drawn off a surface. And the cancel below the spinner made a two-storey block
that the list had to be pushed a long way down to clear.

The redesign, as asked for: a dot fades in as the pull begins, is drawn out into
a tall narrow droplet as it deepens, and pinches off into a rotating dashed ring
at the threshold — one continuous shape, green dashed throughout. The ✕ moves to
the DEAD CENTRE of the ring and the words "Cancel Loading" sit just above it, so
the whole control is ring-sized instead of stacked. It lives in one component
used by every list that pulls, not just Discover.

FR-001, FR-003 and SC-001 below are restated accordingly; everything else stands.

---

## User Scenarios & Testing

### User Story 1 - Pulling to refresh (Priority: P1)

Someone pulls the list down and watches a droplet stretch under their finger,
turn into the spinning ring, and stay — with a cancel in it — until results
arrive.

**Acceptance Scenarios**:

1. **Given** the list is pulled down, **When** the finger moves, **Then** the
   droplet stretches downward rather than merely growing.
2. **Given** the pull passes its threshold, **When** it is released, **Then** the
   droplet gives way to the spinner in the same place.
3. **Given** a search is running, **When** the screen is watched, **Then** there
   is exactly ONE spinner.
4. **Given** the search finishes or fails, **When** it ends, **Then** the ring
   and the cancel both go, leaving nothing behind.

---

### User Story 2 - The cancel belongs to the spinner (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a search is running, **When** the pair is shown, **Then** the ✕ is
   at the centre of the ring and the label sits directly above it.
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

- **FR-001**: The pull indicator MUST be ONE shape throughout: a dot as the pull
  begins, drawn DOWNWARD into a tall narrow droplet as it deepens, closing into
  the ring at the threshold. Its last droplet frame MUST be the ring's first —
  the same circle, so nothing jumps.
- **FR-002**: Exactly one spinner MUST be on screen while a search runs.
- **FR-003**: The cancel ✕ MUST sit at the centre of the ring, with the words
  "Cancel Loading" directly above it, as one block.
- **FR-004**: The pair MUST hold that relationship however the list moves.
- **FR-005**: The pair MUST be the same whatever started the search.
- **FR-006**: The pair MUST remain until the search finishes OR fails.
- **FR-007**: Cancelling a pull-triggered search MUST retract the refresher.
- **FR-008**: The pair MUST still cost no layout.
- **FR-009**: The indicator MUST be drawn in the gap the refresher holds open,
  never over a row.
- **FR-010**: Every list that pulls to refresh MUST use this same indicator.
- **FR-011**: There MUST be no success mark. Finishing is the animation going
  away.
- **FR-012**: The pull MUST be measured from ONE source, so the shape never
  restarts mid-gesture.

## Success Criteria

- **SC-001**: The ✕ is centred on the ring, and the label's bottom edge is above
  the ring's top edge, on the same centre line.
- **SC-002**: A search that errors leaves no refresher open.
- **SC-003**: Through a single pull, the measured content offset never decreases
  before release.

## Credential-Safety Impact

- **None.** Presentation only.

## Assumptions

- The droplet's stretch is driven by the refresher's OWN pull distance, read
  each frame from the transform Ionic applies to the scroller, and redrawn as an
  SVG path. Reading Ionic's transform *and* the browser's rubber-band overscroll
  — which an earlier attempt did — is what made the shape restart once, early in
  the pull: the bounce moves first, then Ionic claims the gesture and resets the
  scroller to zero. One source cannot hand off to itself.
- No `<ion-refresher-content>` is present. Ionic decides between its own JS
  refresher and the browser-native one by inspecting that element's spinners, in
  an async check re-run on every state change — so with one there, the first pull
  can still be mid-decision and swap implementations under the gesture. With no
  content element the check answers false immediately.
