# Feature Specification: Save YouTube music and music videos to the library

**Feature Branch**: `feat/0012-download-youtube-audio`

**Created**: 2026-09-06

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "downloading them as videos, so the user has the option to choose between mp3 and video download for either a single video link, a playlist or the none short videos in a channel ... this is going to be a deployment in Kubernetes ... the main process should be able to start short living pods or jobs, which just start and do what they need to do, and then they die ... we don't necessarily need to keep track of the progress ... whenever the pod or job goes down, then mark it as completed"

## Overview

SynoDL sends things to the NAS to be downloaded, but only what Download Station
itself understands. Music from YouTube does not qualify: it needs an extractor,
it needs converting or muxing, and — the part that actually costs an evening —
it needs to land in the right folder, with the right tags, so a media server
shelves it under the right artist instead of a heap of *[Unknown Album]*.

This feature adds that. The user pastes a YouTube link, picks **Music** or
**Music video**, and SynoDL puts it in the matching library on the NAS, foldered
and tagged so Plex picks it up unattended. The link may be one video, a playlist,
or a channel; for a playlist or channel it takes the real tracks and leaves the
Shorts and promo clips behind, and running the same link again adds only what is
new.

The work itself does not run inside SynoDL. Each request becomes a short-lived
worker job that starts, downloads, and exits. That keeps a long, bandwidth-heavy,
occasionally-hanging task out of the server process entirely, and it means a
failed download can never take the app down with it. Because the user asked for
no progress reporting, there is nothing to stream back: a download shows only
where it is in its life — scheduled, started, then completed or failed — and the
orchestrator, not SynoDL, is the one keeping count.

## User Scenarios & Testing

### User Story 1 - Save a song to the music library (Priority: P1)

Someone finds a song on YouTube, pastes the link into SynoDL, and chooses Music.
Nothing else is asked of them. A little later the song is in the music library as
an audio file, filed under the artist, with cover art, and with its lyrics beside
it — and Plex shows it that way without anyone renaming, retagging, or moving a
file.

**Why this priority**: This is the feature. Everything else is a variation on it,
and on its own it already replaces the manual routine it exists to remove.

**Independent Test**: Submit one video link in Music mode and confirm the audio
file appears at the expected artist/album path with cover art, lyrics alongside
it, and an album tag that is not empty.

**Acceptance Scenarios**:

1. **Given** a valid link to a single video, **When** the user submits it in
   Music mode, **Then** an audio file for that video appears in the music
   library under a folder named for its artist, and inside a folder named for
   its album — or a singles folder when the video declares no album.
2. **Given** that download has finished, **When** the media server scans the
   library, **Then** the track is shelved under a named artist and a named
   album, never under an unknown-album placeholder.
3. **Given** the video publishes captions in its original language, **When** the
   download finishes, **Then** a lyrics file sits beside the audio file, named so
   the media server associates the two.
4. **Given** a link that is not a supported source, **When** the user submits it,
   **Then** the request is refused with a plain explanation and no work starts.

---

### User Story 2 - Save a music video to the video library (Priority: P1)

The same link, the same paste, but the user picks Music video. The video lands in
the separate music-video library at the best quality the source offers, playable
directly on their devices, with subtitles beside it.

**Why this priority**: It is the other half of the choice the user asked for, it
shares the whole pipeline with Story 1, and it is what makes the two libraries
worth separating.

**Independent Test**: Submit the same link in Music video mode and confirm a
video file appears in the music-video library — and nothing appears in the music
library.

**Acceptance Scenarios**:

1. **Given** a valid link to a single video, **When** the user submits it in
   Music video mode, **Then** a video file appears in the music-video library
   under the same artist/album folder shape, and no file is written to the music
   library.
2. **Given** the source offers higher quality only as separate picture and sound,
   **When** the download runs, **Then** the saved video is that higher quality
   with its sound, and neither the picture nor the sound has been re-encoded.
3. **Given** the video publishes captions, **When** the download finishes,
   **Then** a subtitle file sits beside the video, named so the media server
   recognises its language.

---

### User Story 3 - Take a whole playlist or channel, without the filler (Priority: P2)

The user points SynoDL at a playlist or a channel instead of one video. It saves
the actual tracks and skips the Shorts, teasers, and promo clips. Pointing it at
the same playlist or channel again later picks up only what has appeared since.

**Why this priority**: It turns the feature from one-at-a-time into a way to
follow an artist, and the skip rule is what stops a channel from burying the
library in 30-second clips. It depends on Stories 1 and 2 but changes nothing
about how they work.

**Independent Test**: Submit a channel containing both full tracks and short
clips, confirm only the full-length items are saved, then submit the same channel
again and confirm nothing is downloaded a second time.

**Acceptance Scenarios**:

1. **Given** a playlist or channel link, **When** the user submits it, **Then**
   every item long enough to be a real track is saved and every shorter item is
   skipped.
2. **Given** a channel that publishes both long-form videos and Shorts, **When**
   the download runs, **Then** the Shorts are not saved.
3. **Given** an item is a long compilation or continuous mix, **When** the
   download runs, **Then** it IS saved — length only ever excludes items for
   being too short, never for being too long.
4. **Given** the same playlist or channel is submitted again later, **When** the
   download runs, **Then** items already in that library are not fetched again
   and only new items are added.
5. **Given** an item in the playlist is private, deleted, or otherwise
   unavailable, **When** the download runs, **Then** it is skipped and the
   remaining items still download.

---

### User Story 4 - See where a download stands, without babysitting it (Priority: P2)

The user submits a download and gets on with something else. The entry tells them
only what they need: that it is queued, that it has started, and that it has
finished — or that it failed. No percentages, no speeds, no estimates.

**Why this priority**: It is what makes the feature usable rather than a
fire-and-forget shot in the dark, and the deliberately small amount of
information is a requirement in its own right, not a limitation to work around.

**Independent Test**: Submit a download and confirm the entry moves through
scheduled → started → completed; submit a deliberately broken link and confirm
the entry ends at failed rather than completed.

**Acceptance Scenarios**:

1. **Given** a request has been accepted, **When** the user looks at it, **Then**
   it reads as scheduled until the work actually begins.
2. **Given** the work has begun, **When** the user looks at it, **Then** it reads
   as started.
3. **Given** the work has finished successfully, **When** the user looks at it,
   **Then** it reads as completed.
4. **Given** the work ended without succeeding — a bad link, an unavailable
   video, an extractor failure, or a timeout — **When** the user looks at it,
   **Then** it reads as failed and NOT as completed.
5. **Given** a download is in flight, **When** SynoDL itself is restarted,
   **Then** that download still reads correctly afterwards rather than being lost
   or stuck.
6. **Given** any state, **When** the user looks at a download, **Then** no
   percentage, transfer rate, byte count, or completion estimate is shown.

---

### Edge Cases

- **A link to somewhere else entirely.** A link outside the supported sources —
  or a value that is not a link at all — is refused up front, with nothing
  started and nothing written.
- **The same link submitted twice while the first is still running.** The second
  request must not produce a duplicate file or a competing writer in the same
  folder.
- **A download that never ends.** A worker that hangs, or a channel far larger
  than expected, must be stopped by a time limit and reported as failed rather
  than occupying the cluster indefinitely.
- **A worker that cannot write.** If the target library is unmounted, read-only,
  or full, the download fails visibly instead of appearing to succeed.
- **An item with no artist or album information.** It is still saved, filed under
  the channel that published it and a singles folder, never dropped for missing
  metadata.
- **An item with an awkward title.** Emoji, quotation marks, slashes, and very
  long titles must not break the save or produce an unreadable path.
- **A source that changes underneath us.** When the extractor stops working
  against the site, downloads fail and say so; they never report success with
  nothing written.
- **A restart mid-flight.** SynoDL restarting must not orphan a running download
  nor invent a finished one.
- **Two libraries, one mistake.** A music request must never write into the
  music-video library, or the reverse.

## Requirements

### Functional Requirements

- **FR-001**: Users MUST be able to submit a source link together with a choice
  of Music or Music video.
- **FR-002**: System MUST accept links that address a single item, a playlist, or
  a channel, and MUST treat the three consistently in every other respect.
- **FR-003**: System MUST validate the submitted link against an explicit
  allowlist of supported hosts before any work is started, and MUST refuse
  anything else with a plain-language reason.
- **FR-004**: System MUST perform the download in a short-lived worker that is
  created for one request, exits when that request is done, and holds no state
  between requests.
- **FR-005**: System MUST NOT perform the download inside the SynoDL server
  process, and a failing or hanging download MUST NOT affect the availability of
  the rest of the app.
- **FR-006**: Music requests MUST write only into the music library, and Music
  video requests MUST write only into the music-video library.
- **FR-007**: System MUST file every saved item under a folder named for its
  artist and, within it, a folder named for its album — or a designated singles
  folder when the item declares no album.
- **FR-008**: System MUST preserve the item's published title verbatim in the
  saved file name.
- **FR-009**: System MUST write artist and album information into the saved
  file's own tags, consistent with the folders it was filed into, so a media
  server never has to fall back to an unknown-album placeholder.
- **FR-010**: System MUST save cover art with each item.
- **FR-011**: System MUST save the item's captions in its original published
  language, as a companion file named so the media server associates it with the
  item, when such captions exist. An item without captions MUST still be saved.
- **FR-012**: For Music video requests, System MUST save the highest quality the
  source offers and MUST NOT re-encode the picture or the sound.
- **FR-013**: For playlist and channel requests, System MUST skip items shorter
  than a defined minimum length, and MUST NOT exclude any item for being long.
- **FR-014**: For channel requests, System MUST take the channel's full-length
  published items and MUST NOT take its short-form items.
- **FR-015**: For playlist and channel requests, re-submitting the same link MUST
  fetch only items not already saved to that library.
- **FR-016**: System MUST continue a playlist or channel request past an
  individual item that is unavailable, and MUST NOT fail the whole request
  because of it.
- **FR-017**: System MUST expose exactly four states for a download — scheduled,
  started, completed, failed — and MUST NOT expose progress, transfer rate, byte
  counts, or completion estimates.
- **FR-018**: System MUST distinguish a failed download from a completed one; an
  unsuccessful outcome MUST NOT be reported as completed.
- **FR-019**: System MUST report a download's state correctly after a restart of
  the SynoDL server.
- **FR-020**: System MUST stop a download that exceeds a defined maximum running
  time and report it as failed.
- **FR-021**: System MUST NOT retry a download automatically, so that a partially
  completed run is never repeated on its own.
- **FR-022**: Saved files MUST be owned such that the media server and the
  operator can read and manage them without manual permission repair.
- **FR-023**: System MUST refuse or coalesce a request for a link that is already
  being downloaded into the same library, so two workers never write the same
  folder concurrently.
- **FR-024**: System MUST keep a durable record of a download that FAILED, so an
  unsuccessful outcome stays visible after the orchestrator has discarded the
  underlying job. A successful download's entry MAY expire with that cleanup —
  the saved files are its lasting record — but a failure MUST NOT vanish
  unremarked.
- **FR-024a**: A stored failure record MUST be removable by the user, and MUST
  NOT accumulate without bound.
- **FR-025**: The music and music-video libraries MUST be configured once by the
  operator and shared by every SynoDL user. They are instance-wide destinations,
  not per-user ones, and MUST NOT be selectable per request beyond the choice of
  which of the two a given mode writes to.
- **FR-026**: YouTube downloads MUST appear as rows in the existing Tasks list
  alongside NAS downloads, without adding a new tab, and each row MUST be
  visibly marked as to which of the two systems it belongs to.
- **FR-027**: A YouTube download row MUST offer only the actions that apply to
  it, and MUST NOT present controls it cannot honour — there is no pausing,
  resuming, or resource-level control over a worker once it is running.
- **FR-028**: Existing filtering, sorting, and bulk actions over the Tasks list
  MUST continue to behave correctly with both kinds of row present, and MUST NOT
  apply a NAS-only action to a YouTube row.

### Key Entities

- **Download request** — what the user submitted: the source link, the chosen
  mode, the scope implied by the link (single, playlist, or channel), who asked,
  and when.
- **Worker job** — the short-lived unit of work created for one request: which
  request it belongs to, which mode it is serving, and where it is in its life.
- **Media library** — one of the two destinations: which mode writes to it, where
  it is mounted, and the record of what has already been saved into it.
- **Failure record** — the durable trace of a download that did not succeed:
  which link and mode it was, when it failed, and enough of a reason for the user
  to act on it. Removable by the user; not kept for successful downloads.

## Success Criteria

- **SC-001**: A user can go from pasting a link to a saved, correctly filed item
  without renaming, retagging, or moving a single file by hand.
- **SC-002**: Every saved track is shelved by the media server under a named
  artist and a named album; none appear under an unknown-album placeholder.
- **SC-003**: A music video is saved at the source's highest offered quality,
  with picture and sound unchanged from what the source served.
- **SC-004**: Submitting a channel of mixed full-length and short items saves
  every full-length item and no short item.
- **SC-005**: Re-submitting a playlist or channel already saved downloads nothing
  and adds no duplicate files.
- **SC-006**: A download that does not succeed is shown as failed, never as
  completed.
- **SC-007**: A download in flight when SynoDL restarts is still reported
  correctly afterwards.
- **SC-008**: No download can run indefinitely; every one reaches a terminal
  state within its defined time limit.
- **SC-009**: The rest of the app stays responsive while downloads are running,
  including large channel requests.

## Credential-Safety Impact

- **New: SynoDL is trusted to create workloads.** Until now the server held one
  outbound credential, for the NAS. It now also holds the ability to ask the
  orchestrator to run a worker. That ability is a credential in the Principle III
  sense: never logged, never in an error payload, never handed to a client. Its
  permissions MUST be least-privilege and confined to SynoDL's own namespace, and
  MUST NOT extend to reading secrets, executing inside pods, or touching
  workloads that are not SynoDL's.
- **New: a user-supplied value reaches an external process.** The submitted link
  is the first user-controlled value SynoDL passes to something it executes. It
  MUST be validated against the host allowlist before use and MUST reach the
  worker as a discrete argument, never assembled into a command string — the same
  no-open-proxy instinct that governs the DSM allowlist.
- **No new DSM API, and no NAS credential involved.** This feature does not call
  Download Station and does not use the stored NAS connection. The NAS appears
  only as the filesystem behind the two mounted libraries.
- **New writable volumes, worker-only.** The two media libraries are mounted by
  worker pods and by nothing else; the server container never mounts them. They
  hold the operator's media, not SynoDL state, and the single-SQLite-volume rule
  for SynoDL's own state is untouched.
- **In-flight state is not ours to keep.** A download's live state is read back
  from the orchestrator's own record of its jobs rather than mirrored into the
  store, so there is nothing to drift and nothing extra to protect.
- **New: failures are written at rest.** The one thing this feature stores is a
  record of downloads that failed — the submitted link, the chosen mode, the time,
  and a reason. It lives in the existing single SQLite store under the one-store
  rule; it introduces no second datastore. Submitted links are user-chosen public
  URLs rather than secrets, but they are still user data: they stay out of logs
  and metrics exactly as task URIs do, are visible only to the instance's own
  users, and are removable.
- **Still never logged.** Source links, saved file paths, and library paths stay
  out of logs, error strings, and metrics, exactly as task URIs already do.

## Assumptions

- **The download recipe is settled, not open.** The exact extraction, tagging,
  folder, and companion-file behaviour was verified end-to-end against real
  content before this spec was written, including: the original-language caption
  selection, the companion-file naming both media servers actually match, the
  fact that tags rather than folders drive shelving, and the fact that the
  highest offered quality requires combining separate picture and sound streams
  without re-encoding. The plan implements that verified behaviour rather than
  rediscovering it.
- **Only YouTube is in scope.** The host allowlist starts and ends there; other
  sources are a later spec.
- **The two libraries are instance-wide.** Because they are mounted into worker
  pods at deploy time and are the media server's own libraries, they are the
  operator's configuration, not a per-user choice. Anyone who can sign in to this
  SynoDL instance can add to them.
- **Concurrency is bounded by a small operator-invisible limit**, tuned in code
  rather than configured, with requests beyond it waiting rather than failing.
- **A duplicate in-flight request is refused, not queued behind the first**, on
  the grounds that the user asked for the same thing twice and the first is
  already doing it.
- **The minimum length that separates a track from a clip is a constant**, not a
  user setting.
- **The worker image is pinned and updated deliberately.** An outdated extractor
  fails against the source, so its version is reviewed and bumped like any other
  supply-chain dependency rather than floating.
- **Local development and the e2e suite must not require a cluster.** The
  existing mock-DSM parity rule implies the orchestrator needs an equivalent
  stand-in so `make start` and e2e keep working on a laptop; the plan decides its
  shape.
- **No progress reporting is a requirement, not a gap.** Byte-level progress is
  deliberately absent and should not be added later without a new spec.

## Clarifications

### Session 2026-09-06

- Q: How long does a finished download stay visible, and does that mean SynoDL
  stores anything? → A: Record failures only. A successful download fades with
  the orchestrator's own cleanup window — the saved files are its record — but a
  failure gets a durable row so a broken link or an extractor break is still
  visible days later. This is the one piece of stored state the feature adds.
- Q: Are the two media libraries operator-wide or chosen per SynoDL user? → A:
  Operator-wide and shared. They are mounted into worker pods at deploy time and
  are the media server's libraries; per-user destinations would mean per-user
  paths inside a shared mount for no real gain. Everyone who can sign in can add
  to them.
- Q: Where does this live, given the shell already carries five tabs? → A: Mixed
  into the existing Tasks list as ordinary rows, marked by their source. No sixth
  tab and no segmented control. Consequence carried into FR-027 and FR-028: the
  list now holds two kinds of row with different capabilities, so NAS-only
  actions must not be offered on — or applied to — a YouTube row.
- Deferred with a documented default rather than asked (marker limit): worker
  concurrency is a small in-code limit with excess requests waiting, and a
  duplicate in-flight request for the same link and library is refused rather
  than queued. Both are recorded in Assumptions and revisitable in the plan.
