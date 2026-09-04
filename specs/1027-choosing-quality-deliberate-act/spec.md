# Feature Specification: Choosing a quality is a deliberate act

**Feature Branch**: `feat/1027-choosing-quality-deliberate-act`

**Created**: 2026-09-04

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "when I go to a movie that we have, the first option is selected by default, we probably should not show the selection mark by default, and when user selects one quality then we should mark that as selected like a radio button for instance and then scroll down to download section where in case of movies they can send to nas directly, or in case of tv shows seasons they and adjust the episodes they need and then send to nas, also you can drop the toast notification when we added the download to nas, the button at the bottom which shows up after adding is more than enough"

## Context

Opening a title pre-selects an option and marks it. Nothing was chosen, but the
sheet says something was — and the send button is armed to act on it.

Driving the app against a seeded library showed that this is worse than
cosmetic. For a series with seasons 1 and 2 already on the NAS, the sheet opens
on season 3 (the first one missing) but the pre-selection is season 1's first
option, sitting inside a **collapsed** season. The user sees season 3's options,
none of them marked, and a send button reading "Send 4 to NAS" — which would
have fetched a season they already had, without them touching anything.

The same session showed a second defect: sending a season the user does **not**
have still asked "You already have this. Download it again?", because a series
counts as owned when any season is present and the prompt tested the title
rather than the season being sent.

Finally, a successful send raises a toast on top of a button that has just
become a live status control for that very download, saying the same thing
twice.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Nothing is chosen until you choose it (Priority: P1)

A user opens a title. Today an option is already marked and the send button is
ready, as though a decision had been made on their behalf.

After this change nothing is marked and nothing can be sent until the user picks
an option. Picking one marks it, and only it.

**Why this priority**: The pre-selection is not merely presumptuous — on a
part-owned series it is invisible and points at the wrong season, so the primary
action of the screen is armed with something the user cannot see.

**Independent Test**: Open a title; nothing is marked and the send button cannot
be used. Pick an option; it is marked and sending becomes available.

**Acceptance Scenarios**:

1. **Given** a title is opened, **Then** no option is marked and sending is
   unavailable.
2. **Given** the user picks an option, **Then** that option is marked and sending
   becomes available.
3. **Given** an option is picked, **When** the user picks another, **Then** only
   the second is marked.
4. **Given** a part-owned series, **Then** the option that is marked is never one
   the user cannot currently see.

### User Story 2 - The next step comes to you (Priority: P1)

Having picked a quality, the user must find what to do next, which on a long
option list is below the fold.

After this change picking an option brings the next step into view: the episode
selection for a season pack, or the send button for a movie.

**Independent Test**: Pick an option on a many-season series and confirm the view
moves to the episode list without scrolling by hand.

**Acceptance Scenarios**:

1. **Given** a season pack is picked, **Then** the episode selection is brought
   into view and can be adjusted before sending.
2. **Given** a movie option is picked, **Then** the send button is in view and
   the title can be sent directly, with no episode selection shown.
3. **Given** the user changes their pick, **Then** the view follows the new one.

### User Story 3 - Only warn about what is actually a repeat (Priority: P2)

Sending a season the user does not have warns that they already have it, because
a series counts as owned when any of its seasons is present.

**Independent Test**: With seasons 1–2 present, send season 3 — no warning. Send
season 1 — warned.

**Acceptance Scenarios**:

1. **Given** seasons 1–2 are on the NAS, **When** the user sends season 3,
   **Then** no repeat warning appears.
2. **Given** the same, **When** the user sends season 1, **Then** the warning
   appears and names that season.
3. **Given** a movie already on the NAS, **Then** the warning still appears.
4. **Given** a title still arriving, **Then** the warning still appears.

### User Story 4 - One confirmation, not two (Priority: P3)

A successful send pops a toast over a button that has just become a live status
control for the same download.

**Independent Test**: Send something; the button reports the download and no
toast appears.

**Acceptance Scenarios**:

1. **Given** a send succeeds, **Then** no toast appears.
2. **Given** a send succeeds, **Then** the button reports the created download
   and leads to it.

### Edge Cases

- Switching quality tiers must not leave a pick from the old tier marked.
- Collapsing or opening a season must not leave a pick from another season
  marked.
- Sending must remain unavailable while nothing is picked, and for a season pack
  with no episodes ticked.
- Cancelling the repeat warning must send nothing.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: No option MUST be marked when a title is opened.
- **FR-002**: Sending MUST be unavailable until an option is picked.
- **FR-003**: Picking an option MUST mark that option and unmark any other.
- **FR-004**: A marked option MUST always be one the user can currently see;
  where a change hides it, the mark MUST be dropped.
- **FR-005**: Picking an option MUST bring the next step into view — the episode
  selection for a season pack, the send button otherwise.
- **FR-006**: A movie MUST show no episode selection and MUST be sendable
  directly once an option is picked.
- **FR-007**: A season pack MUST allow the episodes to be adjusted before sending.
- **FR-008**: The repeat warning MUST be decided by what is actually being sent:
  for a season pack, whether that season is present; otherwise whether the title
  is.
- **FR-009**: The repeat warning MUST still appear for a title that is arriving.
- **FR-010**: A successful send MUST NOT raise a toast.
- **FR-011**: A successful send MUST leave the button reporting the created
  download and leading to it.

### Key Entities

- **Pick** — the option the user has chosen to send. Absent until chosen, always
  visible while held.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A freshly opened title cannot be sent without a deliberate choice.
- **SC-002**: The marked option is never off-screen.
- **SC-003**: Choosing a quality requires no manual scrolling to reach the next
  step.
- **SC-004**: A season the user does not have is sent without a repeat warning.
- **SC-005**: A season the user has still warns before re-sending.
- **SC-006**: A successful send produces exactly one confirmation.

## Assumptions

- The stored quality preference should still be honoured: it now selects which
  quality tab opens rather than pre-picking an option, so the preference keeps
  its value without making a choice for the user.
- Movies keep warning on title ownership. Owning a different release of a movie
  is still owning the movie, which is the existing designed behaviour.

## Credential-Safety Impact

- Client-only. No API, DSM allowlist, NAS call, persistence or logging changes.
- No new data crosses the boundary; this changes only what is drawn and when a
  send may be initiated.
