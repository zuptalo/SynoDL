# Feature Specification: Every task says where it came from, and an upload says what it is

**Feature Branch**: `feat/1042-tasks-list-sections`

**Created**: 2026-09-09

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "Let's also add a section for From Discover, the last 2 items are from
Discover for instance. Also for the uploads let's show the thumbnail of the Music
or Music Video if available and let's make the rows as rich as the other task
types with title, Artist and Album if they exist, and let's add a detail view for
them just like we have for when items are downloaded from YouTube."

## Overview

The Tasks list has grown four kinds of row and labels only two of them.

**Downloads have no heading.** Uploads say "Uploads" and YouTube downloads say
"From YouTube", and then everything from the NAS just begins — films sent from
Discover sitting under a YouTube playlist with nothing between them. The rows
themselves are fine; it is the seam that is missing.

**An upload row says almost nothing.** A cloud icon, a percentage, and the folder
it went to. Every other row in the list carries artwork, a title and the facts
that identify it, and since spec 1040 an upload knows the track name, the artist
and the album — it just does not show them. It also cannot be opened: it is the
only row in the list with no detail view behind it.

Both are the same omission. The list learned to hold several kinds of thing and
one of them never caught up.

## User Scenarios & Testing

### User Story 1 - Knowing where a download came from (Priority: P1)

Someone scrolling the Tasks list can tell at a glance which downloads came from
Discover and which were added some other way.

**Independent Test**: with a Discover-sent download and a link-added one on
screen, confirm each sits under a heading that names where it came from.

**Acceptance Scenarios**:

1. **Given** downloads sent from Discover, **When** the list is shown, **Then**
   they sit under a "From Discover" heading.
2. **Given** downloads added by link or file, **When** the list is shown,
   **Then** they sit under their own heading rather than under Discover's.
3. **Given** only one kind is present, **When** the list is shown, **Then** only
   that heading appears — a section with nothing in it is not shown.
4. **Given** a filter or sort, **When** it is applied, **Then** it applies within
   the sections exactly as it does today.

---

### User Story 2 - An upload that reads like everything else (Priority: P1)

Someone uploading a track sees a row carrying its artwork, its name, its artist
and its album — the same shape as the download rows beside it.

**Acceptance Scenarios**:

1. **Given** a music upload with a thumbnail, **When** the row is shown, **Then**
   the thumbnail is its artwork.
2. **Given** a music upload with no thumbnail, **When** the row is shown, **Then**
   it falls back to an icon rather than showing a hole.
3. **Given** a music upload, **When** the row is shown, **Then** it names the
   track, the artist and the album where they exist.
4. **Given** a film or episode upload, **When** the row is shown, **Then** it
   reads as it does today — those have no artist or album.
5. **Given** any upload, **When** its state is shown, **Then** it uses the same
   coloured chip every other row uses.

---

### User Story 3 - Opening an upload (Priority: P2)

Tapping an upload opens everything known about it, the way tapping a YouTube
download does.

**Acceptance Scenarios**:

1. **Given** an upload, **When** it is tapped, **Then** a detail sheet opens.
2. **Given** a music upload, **When** the sheet is open, **Then** it shows the
   artwork, track, artist, album, where it was sent and how far along it is.
3. **Given** a failed upload, **When** the sheet is open, **Then** it says why in
   plain language.
4. **Given** the sheet is open while the upload runs, **When** it progresses,
   **Then** the sheet follows it.

---

### Edge Cases

- **A download whose origin cannot be told.** Anything without catalog metadata
  is not from Discover, so it belongs under the other heading rather than being
  guessed at.
- **A thumbnail that is not an image the browser can show.** The row falls back
  to its icon; nothing is left broken.
- **The artwork belongs to the batch, not the file.** A music upload is several
  files with one set of details, so every row of that upload shows the same
  artwork — including the artwork's own row.
- **An upload dismissed while its sheet is open.** The sheet says so rather than
  showing stale data, exactly as a download's does.
- **A very long list.** The headings must not make the list slower or change how
  filtering works.

## Requirements

### Functional Requirements

- **FR-001**: Downloads sent from Discover MUST appear under a heading that says
  so.
- **FR-002**: Downloads added any other way MUST appear under their own heading.
- **FR-003**: A heading MUST NOT appear for a section with nothing in it.
- **FR-004**: Filtering and sorting MUST behave as they do today, within the
  sections.
- **FR-005**: An upload row MUST show the upload's artwork when there is one, and
  fall back to an icon when there is not.
- **FR-006**: An upload row MUST show its track name, artist and album where
  those exist.
- **FR-007**: An upload row MUST use the same state chip the other rows use.
- **FR-008**: An upload MUST open a detail sheet showing everything known about
  it.
- **FR-009**: The sheet MUST follow the upload while it runs, and say so if it is
  dismissed.
- **FR-010**: Artwork MUST come from the file already on the device; nothing is
  read back off the NAS to render a row.

### Key Entities

- **Task origin**: whether a NAS download came from Discover or was added another
  way.
- **Upload batch**: the files sent together as one track, sharing one set of
  details and one piece of artwork.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Every row in the Tasks list sits under a heading naming where it
  came from.
- **SC-002**: A music upload's row shows its artwork, track, artist and album.
- **SC-003**: An upload can be opened and shows the same depth of detail a
  YouTube download does.
- **SC-004**: Rendering an upload's artwork makes no network request.

## Credential-Safety Impact

- **None.** No new stored data, no new endpoint, no new permission. The artwork
  is the file the user just picked, rendered from the device — which is also why
  it costs no request and reaches no server it was not already being uploaded to.

## Notes from implementation

- **The spec had to clean up after itself.** A music upload starts a tagging
  worker (spec 1040), so this is the first non-download spec to leave cluster
  state behind — and it showed up as intermittent failures in the YouTube specs
  rather than in this one. Resetting the mock cluster here is where that belongs.
- **`task-list` had to stay where it was.** The first cut moved that marker onto
  the Discover section, which quietly changed its meaning from "the downloads
  rendered" to "there are Discover downloads" — and half the suite waits on it.
  The two sections sit inside it instead, with markers of their own.

## Assumptions

- A download carrying catalog metadata came from Discover; that is what the
  metadata is for and there is no other marker.
- Upload artwork lives only as long as the upload does. Uploads are already
  client-side and do not survive a reload, so the artwork does not either.
- Music videos use the same shape as music: they have a track, an artist and an
  album too.
