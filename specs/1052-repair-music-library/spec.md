# Feature Specification: Repair the music library: one copy per song, clean names, and real metadata

**Feature Branch**: `feat/1052-repair-music-library`

**Created**: 2026-09-28

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Repair the existing music library at the mounted music volume: one copy per song identified by the YouTube video id every file already carries, playlists preserved as .m3u8 files, Artist/<Album or Singles>/NN - Title.mp3 structure with clean titles and merged artist folders, the colon folder and the .webm/.bin leftovers fixed, and metadata and cover art filled in from MusicBrainz + Cover Art Archive, iTunes Search and Deezer only when the match is confident. Dry-run first; applying moves files and sends removed ones to a .trash folder inside the library. A follow-up spec changes the download recipe."

## Overview

The library on the music volume was filled by downloads, and it is shaped like
the recipe that made it: `Artist / <YouTube playlist title> / <video title>.mp3`.
A survey of the live volume (about 59 GB, 1,888 artist folders, 3,497 playlist
folders, 5,766 tracks) found:

- **The same song stored many times.** 929 track names appear in more than one
  of the same artist's playlist folders — about 1,770 redundant copies. They are
  not byte-identical: the embedded album tag differs per playlist, so a file
  hash cannot recognise them. What every file does carry is the YouTube video
  id, in its `purl` and `comment` tags.
- **Playlist titles standing in for albums** ("Old TikTok Songs That We Forgot
  About", "Hip Hop 00s Playlist ♫ …"), with a made-up year and the genre "Music".
- **Artist folders named after uploaders**, not artists: channels such as
  `10ccVEVO`, `1theK (원더케이)`, `Energy TV`, and near-duplicates such as
  `$uicideboy$` beside `$uicideboy$, Scott Arceneaux Jr, Aristos Petrou`.
- **Leftovers**: `Coldplay/Coldplay: Everyday Life` (a colon, which the mount
  cannot list) beside the clean `Coldplay - Everyday Life`; two `.webm` files that
  were never converted to audio; two `logo.bin` files.
- **Thin tags**: the track title is the video title verbatim, the description is
  embedded as the comment, and there is no track number, and no real album.

This spec repairs what is already there. It is a one-off, reviewable,
reversible operation over one library. Changing how NEW downloads are filed, so
they stop recreating this, is a separate follow-up spec.

## Clarifications

### Session 2026-09-28

- Q: What is a "playlist" when the same title sits under many artists? → A: One playlist per distinct title, library-wide; every artist's folder of that title contributes to a single `Playlists/<title>.m3u8`, ordered by artist then title because the original order is not recoverable.
- Q: How does the operator start it? → A: Operator-run only. One command creates the dry-run Job, which writes a machine-readable plan and a human-readable plan into a `.repair/` folder in the library; a second command naming that plan applies it. No new server endpoint and no UI.
- Q: Which artist folder does a track go in? → A: The lead artist from a confident match (featured artists go in the tags). With no confident match the track keeps its existing uploader folder, cleaned of channel markers such as "VEVO", and lands in `Singles`; nothing is moved to another artist on a guess.
- Q: What counts as a "confident" match? → A: Strict: the artist AND the cleaned title both match exactly after normalising case and punctuation, AND the recording's length is within ±3 seconds of the file's. A near-miss or a mismatched length is "no match".
- Q: Which copy of a song is kept? → A: The copy with the largest file size (least likely to be truncated); ties broken by earliest modified time, then by path.
- Q: What happens to folders that are already real albums (e.g. `Coldplay/Coldplay - Parachutes/`)? → A: A folder named `<Artist> - <X>` whose tracks are all by that artist counts as an album. With no confident match its tracks keep it, as `Artist/<X>/Title.mp3` (no track number is invented), and it still also becomes a playlist file. A confident match to another album wins. Any other old folder is a playlist only, and its unmatched tracks go to `Singles`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See exactly what would change before anything does (Priority: P1)

The operator starts a repair on the library and gets a report — a plan — of
every move, merge, conversion, retag and removal it would make, with the reason
for each and a summary of totals. Nothing on the volume has changed.

**Why this priority**: 59 GB of someone's music is being restructured. Being able to read
and question the plan before it is carried out is what makes the rest safe to
build and safe to run, and it is useful on its own as an audit of the library.

**Independent Test**: Run the dry-run command against a fixture library, compare the
volume before and after byte for byte (nothing differs), and read the plan.

**Acceptance Scenarios**:

1. **Given** a library, **When** the dry run finishes, **Then** every file and
   folder is exactly as it was and a plan naming each intended change exists.
2. **Given** the plan, **When** the operator reads it, **Then** each change
   states its reason (duplicate of X, merged into Y, converted, matched with
   matched by <source>, no confident match) and the totals say how many files, folders
   and bytes are affected.
3. **Given** a dry run that was interrupted, **When** it is started again,
   **Then** it produces a complete plan and never a half-applied one.

---

### User Story 2 - One copy of every song, with its playlists kept (Priority: P1)

Each song exists once. Every playlist it appeared in becomes a playlist file
that lists it, so "what was in Old TikTok Songs" is still answerable and a
media server can still play it as a playlist.

**Why this priority**: it is the biggest win — roughly 1,770 redundant tracks —
and the one whose mistakes cost the most (deleting the wrong song).

**Independent Test**: Apply against a fixture where one video id sits in three
playlist folders. One file remains, three playlist files list it, and the
video's lyrics file sits next to the one kept copy.

**Acceptance Scenarios**:

1. **Given** the same video id in several playlist folders, **When** applied,
   **Then** one file is kept, and each former playlist has a playlist file under
   `Playlists/` listing the kept file by a path relative to the playlist file.
   **Given** folders with the same playlist title under different artists,
   **Then** they become ONE playlist file containing all of those songs.
2. **Given** duplicates that differ in bytes, **When** the plan is made,
   **Then** they are recognised as one song by video id, not by hash or filename.
3. **Given** copies to be removed, **When** applied, **Then** they are moved to a
   `.trash` folder inside the library, not deleted, and can be put back.
4. **Given** a track with a lyrics sidecar, **When** its copy is kept or
   removed, **Then** the lyrics follow the kept audio and are never orphaned.
5. **Given** a file with no video id, **When** planned, **Then** it is kept and
   reported, never merged on a guess.

---

### User Story 3 - A tidy Artist / Album / Track structure with clean names (Priority: P2)

Tracks sit in `Artist / <Album or Singles> / NN - Title.mp3`. Titles lose the
"(Official Music Video)" noise. Uploader-named folders and near-duplicate
spellings fold into one folder per artist.

**Why this priority**: it is what a media server shelves by and what a person browsing
the volume sees, but it depends on the identity work in Story 2.

**Independent Test**: A fixture with `Artist - Song (Official Video).mp3` under
an uploader-named folder ends up as `Artist/Singles/Song.mp3` or, where an album
is confidently known, `Artist/<Album>/NN - Song.mp3`.

**Acceptance Scenarios**:

1. **Given** a title such as "50 Cent - In Da Club (Official Music Video)",
   **When** cleaned, **Then** the title is "In Da Club" and the artist is "50 Cent".
   **Given** "Chief Keef Feat 50 Cent & Wiz Khalifa – Hate Bein' Sober" filed
   under `50 Cent/` and confidently matched, **Then** it moves to `Chief Keef/`
   with 50 Cent and Wiz Khalifa in the tags; **Given** the same file with no
   confident match, **Then** it stays under `50 Cent/Singles/`.
2. **Given** the folders `X` and `X, Y, Z` for the same lead artist, **When**
   merged, **Then** one folder remains and the featured artists live in the tags.
3. **Given** `Coldplay: Everyday Life` beside `Coldplay - Everyday Life`,
   **When** applied, **Then** the tracks are in the clean folder, the colon
   folder is gone, and no name containing a character the mount cannot list is
   ever written.
4. **Given** `Coldplay/Coldplay - Parachutes/` whose tracks have no confident
   match, **When** applied, **Then** they are filed as `Coldplay/Parachutes/Title.mp3`
   (not `Singles`), and `Coldplay - Parachutes` is also a playlist file.
5. **Given** two different videos that would land on the same path (e.g.
   "Easy On Me (Official Video)" and "(Official Lyric Video)"), **When**
   planned, **Then** both are kept and neither overwrites the other: the file
   already there — else the first by video id — keeps the plain name, the other
   is named with its video id, and the clash is reported. Only if that name is
   also taken does nothing move.

---

### User Story 4 - Real metadata and cover art, only where it is trustworthy (Priority: P2)

Where public sources confidently know a track, it gets its album, track number,
year, and MusicBrainz ids, and the album's cover art. Where they do not, it is
left as "Singles" with a clean title and nothing invented.

**Why this priority**: it is what makes the library look like a music library rather than a
pile of video rips; but a wrong match is worse than none, so confidence
gates it.

**Independent Test**: With the three sources answered by fakes, a confident
match is tagged and a not-confident one is left untouched apart from name cleanup.

**Acceptance Scenarios**:

1. **Given** a track MusicBrainz matches confidently, **When** applied, **Then**
   album, track number, year and ids are written to the file and the album cover
   is embedded and saved as the folder image.
2. **Given** MusicBrainz has no confident match, **When** the next source is
   asked, **Then** it is tried in order (iTunes Search, then Deezer) and the
   first confident answer wins.
3. **Given** no source is confident, **When** applied, **Then** the track goes
   to `Singles` with only its cleaned title — no album, year or art guessed.
4. **Given** a source is down, rate-limited or slow, **When** the run continues,
   **Then** the affected tracks are reported as "not looked up" (distinct from
   "no match") so a later run can retry just those.
5. **Given** the plan, **When** it lists a match, **Then** it shows which source
   answered and the artist, title and length comparison that made it confident.

---

### User Story 5 - Leftovers repaired (Priority: P3)

The two `.webm` files become audio or are reported as unconvertible; the two
`.bin` images get their real type or are set aside.

**Why this priority**: it is tiny in volume but each is a file the library cannot play or
show today.

**Independent Test**: A fixture `.webm` with an audio stream converts to a
tagged mp3; one with none is listed in the report and left in place.

**Acceptance Scenarios**:

1. **Given** a `.webm` carrying audio, **When** applied, **Then** an mp3 exists
   with the same identity and naming as any other track, and the original goes
   to `.trash`.
2. **Given** a `.bin` that is really an image, **When** applied, **Then** it is
   renamed with the right extension; if it is not an image it goes to `.trash`.

---

### Edge Cases

- The run is interrupted part-way through applying: nothing is lost, and a
  re-run finishes the job rather than starting a second, different one.
- The volume is written to while the run is going (a download lands): those
  new files are left alone and reported, never moved on a stale plan.
- Two different songs share a clean title and artist: they are different video
  ids, so both are kept, and any name clash is resolved by keeping both.
- A title has no "Artist - " prefix, or the prefix is not the artist: the
  artist comes from the tag or folder and the title is not mangled.
- Artist names in other scripts (Korean, Cyrillic, Persian) are kept as they
  are; only the same artist under two spellings is merged, and only on a
  confident match.
- A name is over the length limit or made only of dots or reserved characters:
  the same sanitising rule downloads use is applied.
- A media-server `.nfo` refers to a path that no longer exists: it is reported
  as orphaned and, if its track is gone, moved to `.trash`; the media server
  rewrites the rest on its next scan.
- Free space on the volume is too low for what the plan needs (a conversion, an
  embedded image): the dry run says so, and apply refuses to start.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repair MUST be started by the operator from the command line,
  with no new server endpoint and no screen in the app. It MUST run in two
  separate steps: a dry run that only
  reads the library and writes a plan, and an apply that carries out that plan.
  The dry run MUST NOT change any file or folder in the library.
- **FR-002**: The plan MUST list every intended change with its reason and give
  totals (files kept, merged, moved, converted, retagged, removed, and bytes
  reclaimed), and MUST be readable by a person without any tool. It MUST be
  written into a `.repair/` folder inside the library as both a machine-readable
  file (which apply consumes) and a human-readable one, and carries an id.
- **FR-003**: Apply MUST be given the id of a plan and carry out only that
  reviewed plan; it MUST refuse to run without one. A step is
  stale when its source file's size or modified time differs from what the plan
  recorded, or its destination now exists; a stale step MUST be skipped and
  reported, never carried out on stale information and never forced.
- **FR-004**: A song's identity MUST be its YouTube video id, read from the
  file's own tags. File hashes and file names MUST NOT be used to decide two
  files are the same song. A file with no readable id MUST be kept and reported.
- **FR-005**: For each song, exactly one audio file MUST remain. The copy kept
  MUST be the one with the largest file size (the least likely to be truncated),
  ties broken by earliest modified time and then by path, so the same library
  always yields the same choice; the plan MUST show which copy was kept and why.
  Its sidecar lyrics file MUST stay with it.
- **FR-006**: Every playlist a song appeared in MUST be written as a `.m3u8`
  file under `Playlists/` listing its songs by path relative to that file, and
  MUST resolve when the library is mounted elsewhere. A playlist is identified
  by its TITLE alone: folders of the same title under different artists are ONE
  playlist, its songs ordered by artist then title (the original order is not
  recoverable from the folders and MUST NOT be invented). Two titles that differ
  only in case or surrounding whitespace are the same playlist. Playlist files
  MUST NOT contain control characters or line breaks taken from a title or path,
  so a hostile title cannot add an entry or a directive.
- **FR-007**: Nothing MUST be permanently deleted. Anything removed goes to a
  `.trash` folder inside the same library, preserving where it came from, and
  MUST be restorable from there. Because retagging rewrites a file in place, the
  journal MUST also record the previous value of every tag it changes, so a
  restore can put the old tags back as well as the old files. A restore whose
  original location is now occupied MUST skip that entry and report it, never
  overwrite. The repair MUST NOT empty `.trash` itself.
- **FR-008**: Tracks MUST end up as `Artist / <Album or Singles> / NN - Title.mp3`.
  `NN` is present only where a confident track number is known. An existing
  album folder (`<Artist> - <X>`, all tracks by that artist) is kept as the album
  `<X>` when a track has no confident match; any other unmatched track goes to
  `Singles`. Titles MUST
  have the "Official Music Video / Official Video / Lyric Video / 4K …" noise
  removed while the original is kept in the track's tags for finding it again.
- **FR-008a**: Two songs (different video ids) that resolve to one file name MUST
  both be kept. The file already at that name — else the first by video id — keeps
  it; every other is named `<name> [<video id>].mp3`. The title TAG is never
  altered by this. The plan MUST report each clash, and when even the video-id
  name is taken it MUST move neither file.
- **FR-009**: A track's artist folder MUST be its lead artist when a source
  matched it confidently, with featured artists kept in the tags (so a track
  filed under an uploader that is not its artist moves to its real artist). When
  no source is confident the track MUST stay in its existing uploader folder,
  cleaned of channel markers ("VEVO", "Official", …), and MUST NOT be moved to
  another artist. Artist folders MUST be merged when they are the same artist,
  including "lead, featured…" variants, using the source's artist identity and
  never on name similarity alone.
- **FR-010**: No name written by the repair may contain a character the volume's
  mount cannot list (including `: * ? " < > |`), be dot-only, or exceed the
  length limit. It MUST reuse the download recipe's own name-sanitising rule, so
  the two cannot disagree about what a safe name is. Every name derived from a
  tag, title or playlist is untrusted input and is sanitised before it becomes a
  path component.
- **FR-011**: Metadata and cover art MUST be looked up from MusicBrainz with
  Cover Art Archive first, then iTunes Search, then Deezer, and the first source
  that is confident wins. A match is confident ONLY when the artist and the
  cleaned title both match exactly after normalising case and punctuation AND
  the recording's length is within ±3 seconds of the file's. A near-miss, or a
  mismatched length, is "no match" and MUST NOT be applied; the track goes to
  `Singles` with a cleaned title only.
- **FR-012**: A confident match MUST write album, album artist, track number,
  year, and MusicBrainz ids into the file, and embed the cover, and save an
  album image beside the tracks.
- **FR-013**: Lookups MUST respect each source's published limits and identify
  SynoDL to it, MUST be cached so a re-run does not ask again for what was
  answered, and a source failing MUST leave the affected tracks reported as "not
  looked up", separate from "no match", and a later run MUST look them up again
  even if they were already filed (a "settled" file whose lookup never
  completed is re-planned, in place, for tags and album only).
- **FR-014**: The repair MUST run as a short-lived worker that mounts exactly one
  library, never in the server process, and MUST NOT add a second datastore. Any
  cache it keeps MUST be derived public facts only — no credentials, no user id.
  The worker MUST have no access to the cluster API and MUST run as the same
  unprivileged user as the download workers.
- **FR-015**: The hosts the repair may contact MUST be their own allowlist —
  MusicBrainz, Cover Art Archive, iTunes Search and Deezer, plus only the hosts
  those services redirect to or serve artwork from (the archive.org hosts behind
  Cover Art Archive, Apple's and Deezer's image hosts) — and MUST NOT be added
  to the catalog image proxy's list. Only HTTPS is allowed; every redirect hop
  MUST be re-checked against the allowlist and the number of hops bounded; a
  host merely ending in an allowed name (`evilarchive.org`) is not allowed. A
  downloaded cover MUST be verified to be an image by its content, not its name
  or reported type, and bounded in size, or it is discarded.
- **FR-021**: Every path in a plan MUST be confined to the library: apply MUST
  reject a plan step whose source or destination is absolute, contains `..`, or
  resolves through a symbolic link, so an edited or corrupted plan cannot move or
  overwrite anything outside the mounted library. The repair MUST NOT follow
  symbolic links while scanning.
- **FR-022**: At most one plan, apply or restore MUST run against a library at a
  time. A second attempt MUST refuse with a clear message, and a lock left by a
  crashed run MUST be recognisable as stale by the operator, never silently
  taken over.
- **FR-016**: `.ytdlp-archive.txt` MUST NOT be modified, so nothing already
  downloaded is fetched again. Media-server `.nfo` files MUST be left alone
  apart from ones whose track no longer exists.
- **FR-017**: A `.webm` file MUST be converted to an mp3 with the same identity
  and naming as any other track if it carries audio, or reported and left if it
  does not. A `.bin` file MUST be given its real image type or set aside.
- **FR-018**: The repair MUST be resumable and idempotent: run twice, the second
  run finds nothing left to do; interrupted, a re-run completes it.
- **FR-019**: Before it starts, apply MUST verify there is enough free space for
  what the plan needs and refuse, with the shortfall, if there is not.
- **FR-020**: Progress and the outcome (counts by kind, anything skipped and why)
  MUST be visible to the operator from the worker's own output and from a
  results file in `.repair/`. Credentials, session ids and full task URLs
  MUST NOT be logged.

### Key Entities *(include if feature involves data)*

- **Song**: one recording, identified by its video id; has a clean title, one
  lead artist, optional featured artists, and at most one kept audio file.
- **Plan**: the reviewable list of intended changes, each with a reason, and the
  totals; made by the dry run and consumed by apply.
- **Playlist**: a titled list of songs, reconstructed from every folder of that
  title across all artists; stored as one `.m3u8` file, ordered by artist then title.
- **Match**: a source's answer for a song — which source, the album, track
  number, year, ids — applied only when it is confident (exact artist and
  title, length within ±3 seconds).
- **Trash entry**: a file set aside instead of deleted, keeping its original
  location so it can be restored.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After apply, no video id appears on more than one audio file, and
  the library holds about 1,770 fewer tracks, with no song lost (every id
  present before is present after, or is in `.trash`).
- **SC-002**: Every distinct playlist title that existed before has one playlist
  file listing exactly the songs the folders of that title held, and every entry
  in it resolves to a real file.
- **SC-003**: A dry run leaves every file and folder unchanged, verified by
  comparing the volume before and after.
- **SC-004**: Running apply a second time makes zero changes.
- **SC-005**: No file or folder name on the volume contains a character the
  mount cannot list, and a full listing of the volume completes with no error.
- **SC-006**: Every track that was given an album has it from a source that was
  confident; no track has an album, year or cover the plan cannot attribute to a
  source, and every other track is under `Singles`.
- **SC-007**: A person can read the plan for a 5,000-track library and decide
  whether to apply it in under 30 minutes.
- **SC-008**: `.ytdlp-archive.txt` is byte-identical before and after.
- **SC-009**: Every outbound request in a full run is to an allowlisted host over
  HTTPS, and none carries anything but the artist, title and a User-Agent naming
  SynoDL; a request to any other host is refused before it is sent.
- **SC-010**: Restoring an applied plan returns every trashed file and every
  retagged file's previous tags, verified by comparing the library before apply
  and after restore.

## Credential-Safety Impact

- **Stored and how it is protected**: nothing is added to SynoDL's store. The repair
  writes plans, results and a lookup cache of public facts (video id → album, year,
  ids) on the media volume; none holds a secret or a user id.
- **Crosses to the NAS**: nothing goes to the DSM API; the worker writes to the
  library through the mount it was given.
- **Outbound**: only the four public sources named in FR-015, over HTTPS. A request
  carries the artist and title being looked up and a User-Agent naming SynoDL — no
  credential, session id, user id or account data.
- **Logs and errors**: counts and library-relative paths only. Tag contents (video
  descriptions can hold personal links), full request URLs and environment variables
  are never logged.

## Assumptions

- The library is the only thing writing to the volume during a run; a
  download landing mid-run is handled (Edge Cases) but not expected.
- `.trash` grows until the operator empties it; nothing purges it automatically.
- The operator will take a snapshot or backup of the volume before the first
  apply; `.trash` protects against a wrong plan, not against a lost volume.
- Every file downloaded by SynoDL carries its video id in its tags; files that
  do not are few, and are kept and reported.
- The media server (Jellyfin/Plex) owns its `.nfo` files and cover images and
  will regenerate them on its next scan of the new structure.
- The four public sources are reachable from the cluster; if one is not, the run
  degrades to the sources it can reach and reports the rest.
- Playlists are reconstructed from the playlist folders, so a song that was in
  a playlist but was never downloaded into that folder is not in its playlist file.
- Changing the download recipe (what new downloads write, skipping songs already
  held, stopping the playlist-as-album naming) is the follow-up spec, and is out
  of scope here.

## Verification (recorded 2026-09-28)

Measured on the live library, read-only (mounted `:ro`, plans written to a scratch folder):

- **Offline plan over the whole library** (5,757 readable tracks, 7,672 other files; 10 minutes): 3,990 distinct
  songs; **1,763 duplicates** set aside by video id (the filename-based survey estimated 1,770), **13.3 GB**
  reclaimable; 3,964 moves, 3,990 retags, 88 playlist files, 3,478 orphaned `album.nfo`, **0 conflicts**, and
  74 same-title/different-video clashes resolved by keeping both (FR-008a). No duplicate destinations, no
  mount-hostile characters and no path escapes across all ~15,000 actions.
- **Real lookups on 60 random tracks** (after retries were added): **27 matched (45%)**, of which **14 (23%)** have
  a known album and cover; 33 (55%) have no confident match; 0 not looked up. Eight of the unmatched had a
  same-song candidate whose length differed by 4–99 seconds. Expect roughly 75% of songs to land in `Singles`
  (offline, before any lookup, 95% do). The ±3 s rule was not loosened; this is the number to judge it by.
- **Real-title cleaning**: video noise left in 198 of 3,990 titles by the first rules, 51 after (the remainder are
  deliberately kept versions such as "Acoustic Version", "Live", "Remix"); no title gained an unbalanced bracket.
- **Known limits of the local check**: 10 files (including `Coldplay/Coldplay: Everyday Life`) were unreadable
  through the AFP mount on this machine — non-ASCII and colon names, the very limit FR-010 exists for. The in-cluster
  plan over NFS is the real check for those.
- **Live services**: `MUSIC_REPAIR_LIVE=1` smoke test passes against MusicBrainz, Cover Art Archive (including the
  two-hop `archive.org` redirect), iTunes Search and Deezer.
- **Leftovers on the real library**: both `.webm` files already have a same-named `.mp3` beside them, so they are
  reported and left; both `logo.bin` files are not images and are set aside.
- **Media server**: whether `Playlists/*.m3u8` is imported automatically is **not verified**.

### Deviations from the first draft, and why

- **FR-005** gained "an already-settled copy wins over a larger new one", so a later run never moves a file a
  previous run placed.
- **FR-008a** (both kept, second named by its video id) replaces "neither moves" for same-title clashes: the real
  library has 74 of them, and leaving both stranded in their old folders was wrong.
- **Atomic retags**: research R9 promised copy-then-rename and the first implementation rewrote tags in place. A
  kill between the rewrite and its journal line stranded the file (both the retag and the move were skipped as
  "changed"); found in review and fixed — tags are now written on a copy and swapped in, the previous values are
  journaled first, and a file carrying this plan's marker is recognised on resume.
- **SIGTERM** is turned into a normal exit so the lock is released and the lookups so far are saved.

### Open decision

Single-artist playlists such as `Adele / 30`, `Billie Eilish / HIT ME HARD AND SOFT` and `Anyma / The End Of
Genesys` are real albums that this version treats as playlists (only `<Artist> - <X>` folders count as albums), so
their unmatched tracks go to `Singles`. 19 single-artist playlists with four or more songs cover up to 288 songs —
but the same rule would also turn `Roya / Persian Dance remix …` (59 songs) into an "album". Not decided; see the
report to the user.
