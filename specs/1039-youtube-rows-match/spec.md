# Feature Specification: YouTube downloads look like the downloads they sit next to

**Feature Branch**: `feat/1039-youtube-rows-match`

**Created**: 2026-09-09

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Reported from use, with screenshots: "The sort order in the tasks tab
doesn't seem to be working, let's also make it more visible when we are in
different states for the youtube download tasks, also if possible please make the
thumbnails for YouTube tasks and subtasks look nice like how they look in the
regular download tasks we have, including their detailed view, please use the
same design and style that we already have established for regular download tasks
in these new youtube related tasks as well."

## Overview

YouTube downloads and NAS downloads share the Tasks list, and spec 0013 was
careful that a track is rendered by the SAME row component as a top-level
download so the two cannot drift apart. Three ways they still have:

**Sorting does not reach them.** The filter sheet's sort and search are applied
by `applyTaskFilter`, which takes NAS tasks. The YouTube section is filtered by
hand — a search term matched against the URL only, and no sort at all — so the
sheet appears to do nothing to half the screen. The search is the worse half: a
row shows its TITLE, and searching for the title it shows finds nothing.

**A state is hard to pick out.** `downloading`, `starting`, `waiting its turn`
and `saved` are all the same small coloured word in the same place. On an
expanded playlist of a few dozen tracks, working out which one is actually
running means reading every row. The list already has a louder idiom for exactly
this — the filled pill a NAS row uses for its media type.

**A thumbnail is letterboxed before it is cropped.** Artwork is fetched at
`hqdefault.jpg`, which is 480×360 — a 4:3 frame with a 16:9 image inside it and
black bands top and bottom. The row's poster slot is 40×60, so the crop keeps
those bands and roughly a third of the width, which is why the tiles read as dark
slivers next to a film poster that fills its slot.

## User Scenarios & Testing

### User Story 1 - Sorting and searching the whole list (Priority: P1)

Someone sorts by name, or types a track title into the search box, and the
YouTube rows respond the way the NAS rows do.

**Independent Test**: with both kinds of download present, change the sort and
confirm the YouTube section reorders; search for a title and confirm the row it
belongs to survives.

**Acceptance Scenarios**:

1. **Given** several YouTube downloads, **When** the sort is set to name,
   **Then** they are ordered by their titles.
2. **Given** a search for text in a row's title, **When** it is applied, **Then**
   that row is kept and unrelated ones are hidden.
3. **Given** a sort by something only a NAS download has — peers, ratio, upload
   speed — **When** it is applied, **Then** the YouTube rows keep a sensible
   stable order rather than an arbitrary one.

---

### User Story 2 - Seeing at a glance what is running (Priority: P1)

Someone opens a playlist of forty tracks and can tell which is downloading, which
are waiting and which are done without reading each line.

**Acceptance Scenarios**:

1. **Given** a list of downloads in several states, **When** it is on screen,
   **Then** each state is legible at a glance and distinguishable by colour.
2. **Given** the same list, **When** compared with the NAS rows above it,
   **Then** the two use the same visual idiom rather than two inventions.

---

### User Story 3 - Artwork that looks like artwork (Priority: P2)

A YouTube row's thumbnail sits in the list looking like the film posters beside
it, and its detail sheet shows a crisp image.

**Acceptance Scenarios**:

1. **Given** any YouTube row, **When** its thumbnail is shown, **Then** it
   carries no black letterbox bands.
2. **Given** a mixed list, **When** it is scrolled, **Then** every row's artwork
   occupies the same slot, so the titles share one left edge.
3. **Given** a detail sheet, **When** it opens, **Then** its artwork is sharper
   than the row's thumbnail rather than the same small image enlarged.

---

### Edge Cases

- **A download with no artwork at all.** A channel publishes no metadata
  document; the row must keep its icon fallback rather than showing a hole.
- **Artwork that fails to load.** The detail sheet asks for a larger image than
  every video has; when it is missing the sheet must fall back rather than break.
- **A status filter narrowed to NAS statuses.** Unchanged: a YouTube download has
  none of them, so a narrowed filter still hides the section rather than
  pretending they match.
- **A state colour in a light theme.** The chip is built from the same tokens the
  existing chip uses, so it follows the theme rather than hard-coding a colour.

## Requirements

### Functional Requirements

- **FR-001**: The Tasks list's sort MUST apply to YouTube downloads.
- **FR-002**: The search MUST match what a row actually shows — its title, its
  artist and the playlist or channel it came from — not only its link.
- **FR-003**: A sort by a property a YouTube download does not have MUST leave
  them in a stable, meaningful order rather than an arbitrary one.
- **FR-004**: A YouTube download's state MUST be legible at a glance and
  distinguishable by colour.
- **FR-005**: The state MUST use the visual idiom the list already has for a
  labelled property, not a new one.
- **FR-006**: A row's thumbnail MUST NOT carry letterbox bands.
- **FR-007**: Every row in the list MUST keep the same artwork slot, so titles
  share one left edge whatever kind of download they are.
- **FR-008**: A detail sheet's artwork MUST be higher resolution than the row's,
  and MUST fall back to what exists when the larger image does not.
- **FR-009**: A download with no artwork MUST keep its existing icon fallback.

### Key Entities

- **Row state**: what a download is doing, rendered as a coloured label.
- **Artwork size**: which of the source's published thumbnail sizes a given
  surface asks for.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Changing the sort visibly reorders the YouTube section.
- **SC-002**: Searching for a title shown on a row finds that row.
- **SC-003**: A row's state is identifiable without reading the row's other text.
- **SC-004**: No YouTube thumbnail in the list shows black bands.
- **SC-005**: The left edge of the titles is the same for both kinds of row.

## Credential-Safety Impact

- **None.** No new stored data, no new secret, no new permission, and no widening
  of the artwork proxy: the larger thumbnail is another path on a host that
  allowlist already covers, requested by the client exactly as the small one is.

## Clarifications

### Session 2026-09-09

- Q: A YouTube thumbnail is 16:9 and the established row rail is a 40×60
  portrait poster. How should it sit? → A: Crop to the poster rail, keeping one
  clean left edge across a mixed list, and fix the real cause of how they look by
  requesting a thumbnail size that is not letterboxed to begin with.
- Q: How should the state read more clearly? → A: A coloured chip — the same
  filled pill a NAS row already uses for its media type, so this is the existing
  idiom applied rather than a second one invented.

## Assumptions

- The source publishes a 320×180 thumbnail for every video and a 1280×720 one for
  most; the first is used in the list and the second in the detail sheet, with
  the first as its fallback.
- Stored artwork URLs are left alone. The size is chosen where the image is
  rendered, so existing records need no migration and a record keeps working if
  the choice changes again.
- YouTube downloads remain their own section rather than being interleaved with
  NAS downloads; sorting applies within the section.
