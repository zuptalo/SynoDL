# Feature Specification: New music downloads write the tidy layout

**Feature Branch**: `feat/1054-new-music-downloads`

**Created**: 2026-09-29

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Change what new music downloads write so they land in the layout the music library repair produces, instead of recreating the old one: no playlist title standing in for the album, playlists as .m3u8 files, one copy of a song by video id, real metadata and cover art only on a strict match, names that are safe on the share, and no stray .webm/.bin files. The repair keeps working and finds nothing to do on a library written this way."

## Overview

Specs 1052 and 1053 repaired the music library: one copy of every song, filed as
`Artist / Album-or-Singles / NN - Title.mp3`, with each playlist kept as a
`Playlists/<name>.m3u8` file. On the live library that removed 1,774 duplicate
copies, filed about 4,000 songs and wrote 88 playlists.

But the thing that made the mess is still running. Every new YouTube download is
written the old way: `Artist / <playlist title> / <video title>.mp3`. The playlist
title is used as the album, so the same song downloaded through two playlists is
stored twice; artist folders are named after whatever channel uploaded the video;
the tags are the video's own title and description; a title with a colon can still
become a folder the share cannot list; and a download can occasionally be left as
`.webm` or `logo.bin`. The repair would have to be run again, and again, to undo
what new downloads keep recreating.

This spec changes what a new **music** download writes, so it lands where the repair
would have put it. A downloaded playlist becomes a playlist file, the songs are
filed by artist and album (or `Singles` when nothing is known for certain), a song
already in the library is not stored a second time, names are safe on the share,
and the tags carry real details only when a match is certain.

Existing libraries are **not** migrated here — the repair does that. Music **video**
downloads are unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A downloaded song lands where the repair would have put it (Priority: P1)

A user downloads a music link. The song is saved as
`Artist / Album-or-Singles / NN - Title.mp3` with a cleaned title and a cleaned
artist name, its tags say the same, and — when the song is a certain match in the
public music databases — it carries the real album, track number, year and cover
art. When no match is certain it is filed under `Singles` with only a cleaned title.
The original YouTube title and link stay in the file so it can always be found back
at its source.

**Why this priority**: this is the whole point. Without it every new download keeps
undoing the repair, and everything else only matters once new songs land in the
right place.

**Independent Test**: download one fixture song with a "(Official Music Video)"
title through the mock stack; assert the resulting path, filename, cleaned tags,
preserved original title and link, and that a repair plan over the library has zero
actions.

**Acceptance Scenarios**:

1. **Given** a video titled "Artist - Song (Official Music Video)", **When** it is
   downloaded, **Then** the file is `Artist/<album or Singles>/NN - Song.mp3`, tagged
   with the clean title and artist, and still carries its original title and link.
2. **Given** a song that is a certain match for a real album, **When** it is
   downloaded, **Then** it is filed under that album with the album's track number,
   year and cover art.
3. **Given** a song with no certain match, **When** it is downloaded, **Then** it is
   filed under `Singles` with a clean title and nothing guessed.
4. **Given** the sources for public metadata are unreachable, **When** a song is
   downloaded, **Then** the download still succeeds and is filed as an unmatched song
   (`Singles`) that a later repair can improve.

---

### User Story 2 - A song already in the library is not stored again (Priority: P1)

A user downloads a playlist, or pastes a link, and some of the songs are already in
the library — from another playlist, an earlier download, or the repaired library.
Those songs are not downloaded or stored a second time. They are only added to the
playlist the user asked for.

**Why this priority**: duplicates were the largest single cost of the old layout
(1,774 removed, tens of GB), and they come back with every playlist that overlaps
one already downloaded.

**Independent Test**: download two fixture playlists that share three songs; assert
each shared song exists once in the library and appears in both playlist files.

**Acceptance Scenarios**:

1. **Given** a library holding song X, **When** a playlist containing X is downloaded,
   **Then** X is not fetched again, no second copy appears, and X is listed in the
   playlist's file.
2. **Given** a library holding song X, **When** the user pastes a direct link to X,
   **Then** they are told it is already in the library and nothing new is stored.
3. **Given** a song the repair kept, **When** the same song is downloaded again,
   **Then** it is recognised as owned — the repair's own identity rule (the video id)
   is what decides.
4. **Given** the user explicitly asks for a fresh copy of an owned song, **Then** it
   is fetched, and never silently overwrites the existing file (see Edge Cases).

---

### User Story 3 - A downloaded playlist or channel becomes a playlist file (Priority: P2)

A user downloads a playlist or a channel. Afterwards, `Playlists/<name>.m3u8` lists
its songs in order, using paths relative to the library so any player and the media
server can follow them. Downloading more of the same playlist later adds to that
file; it never replaces or empties it.

**Why this priority**: it is what replaces "the playlist title is the album", so the
grouping the user cared about survives — but a song is already usable without it.

**Independent Test**: download a fixture playlist, then a second one with an
overlapping song; assert both files exist, the shared song is in both, and the first
file was extended rather than rewritten.

**Acceptance Scenarios**:

1. **Given** a downloaded playlist "Road Trip", **Then** `Playlists/Road Trip.m3u8`
   lists its songs, in order, by library-relative paths.
2. **Given** an existing `Playlists/Road Trip.m3u8`, **When** more songs of that
   playlist are downloaded, **Then** the new songs are added and existing entries are
   kept.
3. **Given** two different playlists whose names differ only by characters the share
   cannot hold, **Then** they do not overwrite each other's file.
4. **Given** a playlist whose title is hostile (path separators, dots only, very
   long, reserved characters), **Then** the file is created safely inside
   `Playlists/`.

---

### User Story 4 - Names can never be the wrong kind of name for the share (Priority: P2)

Whatever a video, channel or playlist is called, the folders and files SynoDL writes
can be listed, copied and opened from any device that reads the share. Colons,
question marks, quotes, angle brackets, pipes and asterisks never appear in a folder
or file name; a trailing dot or space never does either. This is the same rule the
repair applies, and the two are held to one shared table of examples, so they cannot
drift apart.

**Why this priority**: this is the source of the "colon folder" that could not be
read over the share; closing it means the repair does not have to fix it again.

**Independent Test**: run the shared name-case table through the server's naming and
through the repair tool's naming and assert identical results; download a fixture
whose artist and album contain every reserved character.

**Acceptance Scenarios**:

1. **Given** an album called "Everyday Life: Vol. 1", **Then** its folder has no colon.
2. **Given** the same name given to the server and to the repair, **Then** both
   produce the same folder name.
3. **Given** any name containing path separators, dot-only segments, control
   characters or an over-long run, **Then** it cannot leave the library and stays a
   single, bounded path segment (existing safety rules still hold).

---

### User Story 5 - No stray `.webm` or `.bin` files (Priority: P2)

A finished music download is always an `.mp3` (plus its cover art embedded and its
lyrics file when there are lyrics). A download that cannot be turned into an mp3 is
reported as failed, with the reason, instead of leaving a `.webm` or a `logo.bin` in
the library that nothing plays and nothing cleans up.

**Why this priority**: two such files already had to be repaired by hand-written
rules; the cause should be found and removed, not repaired forever.

**Independent Test**: reproduce the cause with a fixture that yields an audio-only
container the converter does not handle by default; assert the outcome is an `.mp3`
or a reported failure, and never a leftover `.webm`/`.bin`.

**Acceptance Scenarios**:

1. **Given** a source whose best audio arrives as `.webm`, **Then** the result is an
   `.mp3`.
2. **Given** a thumbnail in an unexpected format, **Then** no `.bin` (or other
   unrecognised file) is left beside the song.
3. **Given** a download that cannot be converted, **Then** it is reported failed and
   leaves no partial file behind.

---

### User Story 6 - The repair still works, and finds nothing to do (Priority: P3)

An admin runs the library repair over a library that includes songs downloaded the
new way. The check reports nothing to move, retag, deduplicate or rename for them.
Existing operator and Settings workflows behave exactly as before.

**Why this priority**: it is the proof that the two tools agree on what "right"
looks like; it is also the guard against them drifting apart later.

**Independent Test**: download fixtures with the new recipe, then run a repair check
over the resulting library and assert zero actions for those songs.

**Acceptance Scenarios**:

1. **Given** songs written by the new recipe, **When** a repair check runs, **Then**
   it plans zero actions for them.
2. **Given** a library that mixes old-layout and new-layout songs, **When** a repair
   check runs, **Then** only the old-layout songs are planned.
3. **Given** the repair's history and undo, **Then** they behave as before.

---

### Edge Cases

- **Same song, different video** (a lyric video and the official video of one song):
  they have different video ids, so they are different files. Recognising them as the
  same recording is out of scope — the repair's existing rule (video id) is the rule.
- **A deliberate re-download**: a user who really wants a fresh copy of an owned song
  (for example, to replace a damaged file) can ask for it; that is a distinct,
  explicit action, and the resulting file replaces nothing silently — it goes through
  the same "never overwrite" rule the repair uses.
- **Two songs that would file to the same path** (same artist, album and track
  number): both are kept; the later one gets a distinguishing suffix rather than
  replacing the earlier.
- **The library is being repaired while a download finishes**: the repair's single
  run and its lock still apply; a download that finds the library locked waits or
  reports, and never writes half into a plan in flight.
- **A playlist is very large** (thousands of songs): the playlist file is written
  incrementally and safely, and one bad entry never fails the rest.
- **A song already owned appears in a playlist that is later deleted at the source**:
  the song stays; the playlist file stays until the user removes it.
- **Metadata sources are slow or rate-limited**: a download never waits on them
  beyond a bounded time; it files the song as unmatched and moves on.
- **A song that used to be recorded by the old recipe's download archive**: it is
  still recognised as owned, so upgrading never triggers a re-download of what the
  user already has.
- **Music video downloads**: unchanged in every way, including their folder layout.
- **A very long or empty title/artist**: bounded and replaced by a safe placeholder
  exactly as the repair does, never producing an empty or overlong name.

## Requirements *(mandatory)*

### Functional Requirements

**Layout and naming**

- **FR-001**: A new music download MUST be saved as
  `<Artist>/<Album or Singles>/<NN - Title>.mp3` (track number omitted when unknown),
  using the same artist, title and album rules as the library repair.
- **FR-002**: The playlist or channel a download came from MUST NOT be used as the
  album, and MUST NOT appear as a folder in the artist's directory.
- **FR-003**: Artist and title names MUST be cleaned by the same rules the repair
  uses (promotional bracket words removed, featured artists lifted out of the title,
  uploader-style suffixes and channel names not used as the artist folder when a
  better artist is known).
- **FR-004**: Every folder and file name SynoDL writes MUST be free of characters the
  share cannot represent and of trailing dots/spaces, and MUST be a single, bounded
  path segment that cannot escape the library.
- **FR-005**: The server's naming rule and the repair tool's naming rule MUST agree
  on every case in one shared table of examples, and a test in each language MUST
  read that same table.

**One copy per song**

- **FR-006**: A song already in the library (same source video id, the repair's own
  identity rule) MUST NOT be downloaded or stored again by a playlist or channel
  download.
- **FR-007**: A directly pasted link to an owned song MUST tell the user it is already
  in the library and MUST NOT store a second copy unless the user explicitly asks for
  one.
- **FR-008**: A song the old recipe's download archive already records, and a song the
  repair kept, MUST both count as owned, so upgrading never re-downloads them.
- **FR-009**: When two different songs would file to the same path, both MUST be kept
  and neither overwritten.

**Playlists**

- **FR-010**: Downloading a playlist or channel MUST write `Playlists/<name>.m3u8`,
  listing its songs in order by library-relative path.
- **FR-011**: An existing playlist file MUST be merged into, never replaced or
  emptied, and the merge MUST be repeatable without producing duplicate entries.
- **FR-012**: A playlist that overlaps an earlier one MUST list the already-owned
  songs by their existing location.
- **FR-013**: Playlist file names MUST follow FR-004 and MUST NOT let two different
  playlists collide on one file.

**Metadata**

- **FR-014**: A song MUST get real album, track number, year and cover art only when
  it is a strict, certain match in the public sources the repair already uses; a
  song without one is filed as `Singles` with a cleaned title and nothing guessed.
- **FR-015**: Every song MUST keep its original source title and its source link, so
  it can be found back at its source and the repair's identity rule keeps working.
- **FR-016**: Failing to reach the metadata sources MUST NOT fail a download; the song
  MUST be filed as unmatched and remain eligible for a later repair to improve.
- **FR-017**: Metadata lookups MUST use only the hosts the repair tool already allows,
  over HTTPS, with redirects re-checked, and MUST carry no credential, session or
  account information.

**Clean outputs**

- **FR-018**: A finished music download MUST be an `.mp3`; a download that cannot be
  converted MUST be reported failed with a reason and MUST leave no `.webm`, `.bin`
  or partial file in the library.
- **FR-019**: The root cause of the leftover `.webm` and `.bin` files MUST be
  identified and removed, not only cleaned up afterwards.

**Compatibility and boundaries**

- **FR-020**: A repair check over a library written by the new recipe MUST plan zero
  actions for those songs.
- **FR-021**: The repair (operator script and Settings screen), its history and undo
  MUST keep working unchanged.
- **FR-022**: Music **video** downloads MUST be unchanged.
- **FR-023**: Existing libraries MUST NOT be migrated or touched by this change.
- **FR-024**: A music download worker MUST still mount exactly one media library and
  never the other.
- **FR-025**: The worker image MUST stay pinned; user- and source-supplied values MUST
  stay host-allowlisted, passed as discrete arguments, and sanitised before they
  become part of a path.
- **FR-026**: The server MUST still never mount the library and MUST still learn about
  a download only from its worker and the bounded, parsed output it produces; that
  output MUST stay bounded, parsed to a fixed shape, never logged and never returned
  raw.
- **FR-027**: No new permission for the server, no new datastore and no credential
  MAY be introduced.
- **FR-028**: The Downloads list, its states, progress and notifications MUST behave
  as they do today; the only user-visible differences are where the file lands and,
  for an owned song, the "already in your library" message.

### Key Entities *(include if feature involves data)*

- **Library song**: one audio file, identified by its source video id, filed as
  artist / album-or-Singles / numbered title, carrying clean tags plus the original
  title and link.
- **Owned set**: the video ids the library already holds — from the songs on the
  share and from the old download archive — used to decide what not to fetch again.
- **Playlist file**: `Playlists/<name>.m3u8`, an ordered list of library-relative
  song paths, merged into over time.
- **Naming rule**: the single set of rules (with one shared example table) that turns
  any source-supplied name into a safe path segment.
- **Download result**: the finished outcome of a download as the worker reports it —
  which songs were stored, which were already owned, which failed and why — bounded
  and parsed, never raw.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After downloading any set of music links, a repair check over the
  library plans **zero** actions for the newly downloaded songs.
- **SC-002**: Downloading two playlists that share N songs stores each shared song
  **exactly once** and lists it in **both** playlist files.
- **SC-003**: **No** folder or file created by a new music download contains a
  character the share cannot represent, across the shared name-case table and a
  download whose names use every reserved character.
- **SC-004**: **Zero** `.webm`, `.bin` or partial files remain after any music
  download, successful or failed.
- **SC-005**: A song that is not a certain match is never given a guessed album: in a
  fixture set of certain and uncertain songs, **100%** of uncertain ones are filed
  under `Singles`.
- **SC-006**: With the metadata sources unreachable, **100%** of downloads still
  complete, filed as unmatched.
- **SC-007**: Upgrading re-downloads **nothing** the user already owns: songs recorded
  by the old archive or kept by the repair are all recognised.
- **SC-008**: The server's and the repair tool's naming agree on **100%** of the shared
  example table.
- **SC-009**: Music video downloads produce byte-for-byte the same layout as before.

## Credential-Safety Impact

- **What is stored or protected**: nothing new. No new state in SynoDL's single
  store; the library and its playlist files stay on the media volume. The old
  download archive is only read to decide ownership.
- **What crosses to third parties**: song title and artist text go to the public
  metadata sources the repair already uses (the fixed host list, HTTPS only, redirects
  re-checked). No credential, session id, user id or account data is sent.
- **What is written to logs and errors**: counts and library-relative paths only.
  Video descriptions, cookies, full URLs with query strings and environment values are
  never logged, and the worker's output the server reads stays bounded, parsed and
  never returned raw.
- **Why the boundary holds**: the download worker still mounts exactly one library,
  the server still mounts none, the worker image is still pinned, source-supplied
  values are still allowlisted, passed as discrete arguments and sanitised into safe
  path segments, and no server permission is widened.

## Assumptions

- The repair's rules (identity by video id, cleaned titles and artists, strict match
  before album metadata, `Singles` otherwise) are the reference; this spec brings
  new downloads to them and does not change them.
- The layout applies only to **music** downloads; music videos keep their layout.
- The share's existing data is left to the repair; a mixed library (old and new
  layout) is expected for a while and is handled by the repair as it is today.
- The public metadata sources and their limits are the ones the repair already uses;
  no new outbound host is added.
- Recognising the same recording under different video ids is out of scope.
- What the user sees for an owned song (a message versus a silent skip) is decided in
  clarification, but it must never be a failure.
- How much of the repair's code the download shares with it is a planning decision;
  the outcome to preserve is that the two cannot disagree.
