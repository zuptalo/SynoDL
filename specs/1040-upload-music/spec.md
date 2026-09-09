# Feature Specification: Uploading music and music videos into the library

**Feature Branch**: `feat/1040-upload-music`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: "Let's add Music and Music Videos to the upload options as well, we
should be asking for Track Name, Artist and Album and an Optional Thumbnail for
the music upload, in case multiple files are added for music and video we should
use the provided values for them all, if it is not too challenging to bake the
data into the video uploads let's do that, but if it is let's just use the
provided details to name the media file right and create the required folders if
missing."

## Overview

Uploading (spec 1022) can put a film or an episode into the library. A track
cannot go anywhere: the sheet offers Movie and TV show, the file-type allowlist
holds no audio format at all, and — the part that actually blocks it — **SynoDL
knows no NAS path for music**.

That last one is worth stating plainly, because it is not an oversight. The music
and music-video libraries exist today only as Kubernetes PVC claim names that a
download worker mounts (spec 0012). The server never mounts them and a claim name
is not a NAS path, so there is nowhere for an upload to be written. Movie and TV
parents come from the download sources, and a music library has no source.

So this adds three things: somewhere for music to go, a way to describe a track,
and — because a media server shelves by tags as well as by folders — a way to get
those descriptions into the file itself.

The folder layout is not a new decision. Spec 0012's shipped recipe files a
downloaded track as `Artist / Album / Track.ext`, falling back to `Singles` when
the source publishes no album, and a music video the same way in its own library.
An upload composes exactly that, so an uploaded track and a downloaded one are
indistinguishable afterwards — the same rule spec 1022 already holds for films.

## User Scenarios & Testing

### User Story 1 - Sending a track to the library (Priority: P1)

Someone picks an audio file, says what the track is called, who it is by and
which album it belongs to, and it lands where the media server will find it.

**Independent Test**: with a music library configured, upload one audio file with
all three fields and confirm it appears at `Artist/Album/Track.ext` with the
folders created.

**Acceptance Scenarios**:

1. **Given** a track name, artist and album, **When** a file is uploaded,
   **Then** it is written as `Artist/Album/Track.<its extension>`.
2. **Given** an artist or album folder that does not exist, **When** the upload
   runs, **Then** it is created.
3. **Given** an artist folder that already exists, **When** the upload runs,
   **Then** the track joins it rather than a near-duplicate being made.
4. **Given** no album is given, **When** the upload runs, **Then** it is filed
   under `Singles`, exactly as a download with no album is.

---

### User Story 2 - The files that belong with a track (Priority: P1)

Someone picks the audio, its lyrics and a thumbnail together, and all three land
correctly named beside each other.

**Why this priority**: a media server matches a lyrics file to its audio **by
identical base name**. Uploading them under two different names is the same as
not uploading the lyrics at all.

**Acceptance Scenarios**:

1. **Given** an audio file and a `.lrc` picked together, **When** they are
   uploaded, **Then** both are named after the track and differ only in
   extension.
2. **Given** a thumbnail is picked, **When** it is uploaded, **Then** it becomes
   the album's cover art rather than a stray image.
3. **Given** several files, **When** they are uploaded, **Then** all of them use
   the one set of details that was entered.

---

### User Story 3 - A music video (Priority: P2)

The same, into the music-video library rather than the music one.

**Acceptance Scenarios**:

1. **Given** the music-video kind, **When** a video file is uploaded, **Then** it
   is filed by the same `Artist / Album / Track` shape in the music-video
   library.
2. **Given** an audio file, **When** the music-video kind is chosen, **Then** it
   is refused — and the other way round.

---

### User Story 4 - The details end up in the file (Priority: P2)

After the file lands, its own tags say what it is, so a media server that reads
tags agrees with the folders.

**Why this priority**: it is the part the operator asked for "if it is not too
challenging", and it is separable — a track filed correctly is already usable.

**Independent Test**: upload a track and confirm a tagging worker is started for
it carrying exactly the entered values.

**Acceptance Scenarios**:

1. **Given** a successful music upload, **When** it completes, **Then** the
   track's title, artist and album are written into the file.
2. **Given** a thumbnail was uploaded, **When** tagging runs, **Then** it is
   embedded as cover art.
3. **Given** tagging fails, or cannot run at all, **When** the upload is
   reviewed, **Then** the upload still counts as succeeded and the file is still
   correctly placed.
4. **Given** no orchestrator, **When** a track is uploaded, **Then** it lands
   normally and nothing is tagged.

---

### Edge Cases

- **No music library configured.** The Music and Music video options must not be
  offered at all, rather than offered and then failing.
- **A track, artist or album name that is not a safe folder segment.** Anything
  containing a separator, a parent reference, or nothing but dots must not be
  able to escape the library — the same guard a download's metadata already gets.
- **Two tracks with the same name in one album.** Uploading the second must
  report a collision rather than silently overwrite, exactly as a film does.
- **A file whose type does not belong.** A `.mkv` in a music upload, or an `.mp3`
  in a music-video upload, is refused before any byte is written.
- **The PVC and the NAS path disagree.** If the configured music path is not the
  library the worker mounts, tagging finds no file. The upload is unaffected.

## Requirements

### Functional Requirements

- **FR-001**: An administrator MUST be able to configure the NAS path of the
  music library and of the music-video library.
- **FR-002**: An upload kind whose library is not configured MUST NOT be offered.
- **FR-003**: A music upload MUST take a track name, an artist and an optional
  album, and MUST accept an optional thumbnail.
- **FR-004**: The destination MUST be composed by the SERVER from those values
  and a parent it already holds. A client MUST never supply a path.
- **FR-005**: The layout MUST be `Artist / Album / Track.<extension>`, with
  `Singles` when no album is given — the same shape a downloaded track gets.
- **FR-006**: Missing folders MUST be created; existing ones MUST be joined.
- **FR-007**: Every file in one upload MUST use the one set of details entered.
- **FR-008**: The audio and its lyrics MUST share a base name, so a media server
  can match them.
- **FR-009**: A thumbnail MUST become the album's cover art under the name a
  media server looks for.
- **FR-010**: Each kind MUST accept only the file types that belong to it: audio
  and its sidecars for music, video and its sidecars for a music video.
- **FR-011**: Every client-supplied value that becomes a path segment MUST be
  sanitised so it cannot escape the library, and MUST be refused rather than
  repaired where it cannot be made safe.
- **FR-012**: After a successful music or music-video upload the entered details
  SHOULD be written into the file's own tags, and the thumbnail embedded as cover
  art.
- **FR-013**: Tagging MUST be best-effort: a failure, or no orchestrator at all,
  MUST NOT change the outcome of the upload or the placement of the file.
- **FR-014**: Tagging MUST run as a short-lived worker mounting exactly one media
  library, with every supplied value passed as a discrete argument.

### Key Entities

- **Music library parent**: the NAS folder that holds the music library, and its
  music-video counterpart.
- **Track description**: a track name, an artist, an optional album, an optional
  thumbnail — one set per upload, applied to every file in it.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A track uploaded with all three fields is at
  `Artist/Album/Track.ext` and needs no manual tidying.
- **SC-002**: Lyrics uploaded with a track are matched to it by the media server.
- **SC-003**: An upload cannot write outside the configured library, whatever is
  typed into the three fields.
- **SC-004**: A tagging failure leaves an upload reported as succeeded.

## Credential-Safety Impact

- **New stored data**: two NAS paths on the existing single-row operator config.
  Not secrets; no new datastore.
- **The upload boundary is unchanged**: the client still supplies no path. Three
  more client strings reach a path segment and all three go through the same
  sanitising-or-refusing rule the file name already gets (FR-011).
- **A second kind of worker**: the tagging Job mounts exactly ONE media library,
  takes its values as discrete argv elements never interpolated into a shell
  string, uses the same pinned image, and needs no new permission — the existing
  namespaced Role already creates Jobs.
- **Nothing new is logged.** A track name is not a secret, but the existing rule
  against logging user content is unchanged.

## Clarifications

### Session 2026-09-09

- Q: Where should the NAS paths for the music libraries be configured? → A: An
  administrator setting in the app, stored beside the NAS connection, so a path
  can be changed without editing a manifest or restarting.
- Q: When several files are picked, what are they? → A: One track and its
  companions — audio, lyrics, artwork. All take the track's name as their base,
  which is how a media server matches a lyrics file to its audio.
- Q: Can the details be baked into the file? → A: Yes, but not during the upload,
  which is streamed straight through and never held on the server. A short-lived
  worker tags it afterwards — the same pattern, image and mounted library the
  YouTube downloads already use.

## Assumptions

- The configured music NAS path and the library the worker mounts are the same
  place. They have to be for a downloaded and an uploaded track to sit together,
  and if they are not, tagging simply finds nothing while the upload is
  unaffected.
- Cover art is written as `cover.<ext>` in the album folder, which is what both
  common media servers look for.
- One upload describes one track. Uploading a whole album is several uploads, and
  a bulk album form is out of scope here.
