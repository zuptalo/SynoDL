# Feature Specification: A track inside a playlist looks like any other download

**Feature Branch**: `feat/1037-item-rows-and-polish`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "Let's make sure each sub task gets its own thumbnail and the nice
details we show on the parent level as well for its own track, we should be able
to tap a url in any level and have it copied to the clipboard, also let's add a
paste button to from youtube view and add proper padding in the design so
boundaries are not so close to the contents"

## Overview

Spec 0013 made a playlist expand into its tracks, and then rendered those tracks
as a plainer, poorer thing than the row they came from: a title, a state, and
nothing else. No artwork, no artist, no marker saying what it is. A track IS a
download — it has its own progress, its own outcome, its own retry — and it
should look like one.

Alongside that, three smaller things the same screens need: a link should be
copyable by tapping it, the YouTube sheet should offer a paste button like the
general one already does, and the sheets need room to breathe.

## User Scenarios & Testing

### User Story 1 - A track looks like a download (Priority: P1)

Opening a playlist shows its tracks the way the Tasks list shows anything else:
artwork, who published it, what it is being saved as, where it has got to.

**Why this priority**: it is the substance of the request, and the current row is
conspicuously poorer than the one beside it.

**Independent Test**: expand a playlist and confirm each track carries artwork
and the same fields the group row does.

**Acceptance Scenarios**:

1. **Given** an expanded playlist, **When** its tracks are shown, **Then** each
   carries its own artwork.
2. **Given** a track, **When** it is shown, **Then** it carries the same details
   as a top-level download — source marker, artist, mode, state and progress.
3. **Given** a track whose artwork cannot be loaded, **When** it is shown,
   **Then** the row falls back to an icon rather than a gap.
4. **Given** a track, **When** it is swiped, **Then** the actions that apply to
   it are offered, as they are for any other download.

---

### User Story 2 - Copying a link (Priority: P2)

Tapping a link copies it, so it can be pasted somewhere else without retyping.

**Independent Test**: open a download's details, tap the link, confirm the
clipboard holds it.

**Acceptance Scenarios**:

1. **Given** any download's details, **When** the link is tapped, **Then** it is
   copied and the app says so.
2. **Given** a clipboard that cannot be written, **When** the link is tapped,
   **Then** the app says so rather than appearing to succeed.

---

### User Story 3 - Pasting a link (Priority: P2)

The YouTube sheet offers a paste button, as the general new-task sheet does.

**Acceptance Scenarios**:

1. **Given** a link on the clipboard, **When** paste is tapped, **Then** the box
   holds it and the detected-link count updates.
2. **Given** a clipboard that cannot be read, **When** paste is tapped, **Then**
   the app explains and suggests pasting by hand.

---

### User Story 4 - Room to breathe (Priority: P3)

Content is not pressed against the edges of a sheet.

**Acceptance Scenarios**:

1. **Given** any of this feature's sheets, **When** it is shown, **Then** its
   content is inset from the edges consistently.
2. **Given** a device with a home indicator or notch, **When** a sheet is shown,
   **Then** its content clears the safe area.

---

### Edge Cases

- **A track with no artwork.** Artwork is derived from the item's identity rather
  than fetched, so it is always addressable — but the image may still 404, and
  the row must fall back rather than show a hole.
- **A clipboard that refuses.** Reading it is restricted on iOS; both directions
  must fail in words rather than silently.

## Requirements

### Functional Requirements

- **FR-001**: A track of an expanded playlist or channel MUST carry its own
  artwork.
- **FR-002**: Artwork for a track MUST be derived from the track's own identity,
  not fetched per item at expansion time — expansion has no ceiling, and one
  lookup per entry would make a large channel slow and rate-limited.
- **FR-003**: A track MUST be rendered with the same row treatment as a top-level
  download, so the two cannot drift apart.
- **FR-004**: A track's row MUST offer the same actions a top-level download's
  row offers, subject to the same rules about which apply.
- **FR-005**: Tapping a link in a download's details MUST copy it and confirm
  that it did.
- **FR-006**: A failure to copy MUST be reported, never silently swallowed.
- **FR-007**: The YouTube sheet MUST offer a paste button, and MUST explain when
  the clipboard cannot be read.
- **FR-008**: Sheets MUST inset their content from the edges and clear the
  device's safe area.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Every track of an expanded playlist shows artwork and the same
  fields as the row it came from.
- **SC-002**: Expanding a playlist of any size makes no additional per-item
  network request to obtain artwork.
- **SC-003**: A link can be copied from a download's details in one tap.
- **SC-004**: A link can be pasted into the YouTube sheet in one tap.

## Credential-Safety Impact

- **No new value reaches a worker, and nothing new is stored.** Artwork is
  derived from an id SynoDL already holds and is fetched through the existing
  artwork proxy, whose host allowlist is unchanged.
- **Clipboard access is a browser capability, not a server one.** Reading it
  happens only on an explicit tap, and what is read goes into a form field the
  user can see — it is never sent anywhere on its own.

## Assumptions

- The source's thumbnail address is derivable from the item's identity, as it is
  for the top-level rows that already show artwork.
- Row treatment is shared by reusing the existing row component rather than
  copying its markup, so a change to one cannot leave the other behind.
