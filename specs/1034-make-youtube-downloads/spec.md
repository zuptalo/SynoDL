# Feature Specification: Make YouTube downloads readable in the task list

**Feature Branch**: `feat/1034-make-youtube-downloads`

**Created**: 2026-09-07

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "also the task title and thumbnail and basically the whole row for youtube tasks maybe can be made a bit more user friendly"

## Overview

A YouTube download in the task list reads `youtube.com/watch?v=JDS7zS7_DwE`
beside a generic note icon. The NAS download directly beneath it shows a poster,
a title, a year and a rating. One of those rows tells you what it is.

The cause is structural rather than cosmetic. Spec 0012 deliberately learns
nothing about a download: the worker does everything and reports back only where
it is in its life. That was the right call for *state* — it is why a restart
cannot desynchronise anything — but it left the row with nothing to render
except the address the user pasted.

The fix is small and does not disturb that. The source publishes a title and an
uploader at a public address, for free, with no key and no quota. Fetching that
once when the request is accepted, and keeping it beside the request rather than
in a database, turns the row into a description of a *thing* instead of a URL —
without the server learning anything about the download's progress, and without
a second datastore.

## User Scenarios & Testing

### User Story 1 - Know what is downloading (Priority: P1)

Someone submits a track and the row names it: the song, and who published it.
When they come back later they can tell one download from another without
opening YouTube to decode a video id.

**Why this priority**: It is the whole complaint. A list of identical-looking
URLs is not a list.

**Independent Test**: Submit a link and confirm the row shows the video's title
and uploader rather than the address.

**Acceptance Scenarios**:

1. **Given** a submitted video link, **When** the row appears, **Then** it shows
   the video's own title rather than the URL.
2. **Given** a submitted video link, **When** the row appears, **Then** it names
   who published it.
3. **Given** the source cannot be asked, or answers with nothing useful,
   **When** the row appears, **Then** it falls back to the readable form of the
   link and the download proceeds exactly as before.
4. **Given** a playlist or channel link, **When** the row appears, **Then** it
   names the playlist or channel — not one track, which would be a lie about
   what is being fetched.

---

### User Story 2 - Recognise it at a glance (Priority: P2)

The row carries the video's own artwork in place of the generic icon, so a
YouTube download reads at the same glance-speed as the NAS downloads around it.

**Why this priority**: It is the smaller half of the complaint and carries the
larger design question, so it should not hold up the title.

**Independent Test**: Submit a link and confirm the row shows the video's
artwork, and that a video without artwork still renders cleanly.

**Acceptance Scenarios**:

1. **Given** a download whose artwork is known, **When** the row appears,
   **Then** it shows that artwork where the placeholder icon was.
2. **Given** artwork that fails to load, **When** the row appears, **Then** it
   falls back to the icon rather than leaving a gap.
3. **Given** any download, **When** its artwork is fetched, **Then** the
   viewer's browser does not contact the artwork's host directly.

---

### Edge Cases

- **A slow or unreachable metadata source.** Looking up a title must never
  delay accepting the request, and must never be the reason a download does not
  start.
- **A very long title.** It must not push the row's status line out of shape or
  wrap it onto several lines.
- **A title in a non-Latin script.** It must render as published, in its own
  direction, without breaking the row's layout.
- **A private or removed video.** Metadata will be unavailable while the
  download itself will also fail; the row must not end up blank, saying neither
  what it was nor what happened.
- **The same video submitted as both audio and video.** Two rows, same title —
  they must remain tellable apart by their mode.

## Requirements

### Functional Requirements

- **FR-001**: When a download is accepted, System MUST attempt to learn the
  item's title and publisher from the source.
- **FR-002**: That attempt MUST be time-bounded and MUST NOT delay acceptance of
  the request beyond a brief limit.
- **FR-003**: Failure to learn anything MUST NOT prevent, delay, or fail the
  download.
- **FR-004**: A row MUST show the item's title when known, and MUST fall back to
  a readable form of the link when not.
- **FR-005**: A row MUST show who published the item when known.
- **FR-006**: For a playlist or channel, the name shown MUST describe the
  playlist or channel rather than any single item within it.
- **FR-007**: A row MUST show the item's artwork when known, and MUST fall back
  to the existing placeholder when it is absent or fails to load.
- **FR-008**: Artwork MUST be fetched by the server on the client's behalf; a
  viewer's browser MUST NOT contact the artwork host directly.
- **FR-009**: The proxy that fetches artwork MUST accept only hosts that serve
  this feature's artwork, and MUST NOT be widened to accept arbitrary hosts.
- **FR-010**: What is learned MUST NOT be written to the application's database.
- **FR-011**: A long or non-Latin title MUST NOT alter the row's height or push
  its status line out of shape.
- **FR-012**: Nothing about a download's progress may be learned or shown; this
  feature adds description only.

### Key Entities

- **Download request** — gains a description: the item's own title, who
  published it, and where its artwork lives. All three are best-effort and
  absent more often than a database column would be.

## Success Criteria

- **SC-001**: A user scanning the task list can name each YouTube download
  without opening anything.
- **SC-002**: A download whose metadata could not be fetched still runs, and
  still reports its state correctly.
- **SC-003**: Submitting a download feels no slower than before.
- **SC-004**: No request from a viewer's browser goes to the artwork host.
- **SC-005**: A playlist download never displays the name of a single track as
  though it were the whole request.

## Credential-Safety Impact

- **New: an outbound call to a public metadata address.** The server contacts a
  public, unauthenticated endpoint on a host already governed by this feature's
  allowlist. It sends no credential, no session, and no user identity — only the
  link the user submitted, which is a public address by definition. The call is
  time-bounded and its response size is bounded, so a hostile or broken endpoint
  cannot hang or exhaust the server.
- **The allowlist still runs first.** The link is validated against the existing
  host allowlist BEFORE any metadata call is made, so this cannot be used to
  make the server fetch an arbitrary address.
- **New: an artwork proxy, deliberately narrow.** Artwork is fetched
  server-side so a viewer's browser never contacts the artwork host — the same
  reasoning behind the existing catalog poster proxy. It gets its OWN host rule
  rather than widening that proxy's: those hosts are declared by download-source
  drivers and by operator-configured mirrors, and YouTube is neither. Sharing
  one list would mean an operator editing a source could change what this
  feature may fetch, and vice versa.
- **Nothing new is stored.** What is learned lives beside the request in the
  orchestrator, exactly as the submitted link and submitter already do. No
  table, no column, no migration.
- **Nothing new is logged.** Titles and artwork addresses stay out of logs,
  error payloads and metrics, as task URIs already do.

## Assumptions

- **Description, not progress.** This adds only what a download IS. Byte-level
  progress remains deliberately absent (spec 0012) and this spec does not
  reopen it.
- **Best-effort throughout.** Every field here may be missing, and the row is
  designed for that rather than treating absence as an error.
- **The lookup happens once, at submission.** A title does not change, so there
  is no refresh, no polling, and no cache to invalidate.
- **Only the already-allowlisted source is consulted.** No third-party metadata
  service is introduced.
