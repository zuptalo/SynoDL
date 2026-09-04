# Feature Specification: Name new downloads the way Plex expects

**Feature Branch**: `feat/1021-name-new-downloads`

**Created**: 2026-09-03

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator feedback after tidying a real library into the Plex/Jellyfin
convention: "can you make sure that SynoDL also follows the plex convention when adding new
movies and tv shows as well so we keep things tidy?"

## Overview

SynoDL names a download's destination folder after the raw catalog title, so a send produces
`movie/Despicable Me 4 2024` and `tv-show/Friends 1994 - 2004`, with every season of a series
landing flat in that one folder. That is not the convention media servers scrape. Plex,
Jellyfin and Emby expect `Movie Title (Year)` and `Show Title (Year)/Season NN/`.

An operator who has tidied their library into that shape gets it undone one download at a time:
every new send re-introduces the old form beside the corrected ones. This changes what SynoDL
writes, so a tidy library stays tidy.

**This is a change to a persistence key, not only to a display name.** The destination folder is
how a download is matched back to its catalog metadata, its owner, and its size on completion.
Nothing may lose those links.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A movie's destination folder MUST be named `Title (Year)` — the catalog title with
  its release year moved into parentheses at the end.
- **FR-002**: A series or anime destination MUST be `Show Title (Year)/Season NN`, where `NN` is
  the two-digit season the downloaded files belong to.
- **FR-003**: Where a catalog title carries a year RANGE (`Friends 1994 - 2004`, `Severance 2022 -`),
  the folder MUST use the FIRST year, which is what a scraper keys a show on.
- **FR-004**: A title carrying no year MUST keep its name unchanged rather than gain empty
  parentheses, and a title that IS a year (`1917`) MUST NOT be emptied.
- **FR-005**: A title whose year is already parenthesised MUST NOT gain a second set.
- **FR-006**: When the season cannot be determined, the files MUST go in the show's folder
  without a season subfolder rather than into a guessed one.
- **FR-007**: The season MUST be derived from what is actually being downloaded, not from a
  value the client supplies, so it cannot disagree with the files.

### Preserving what the destination is used for

- **FR-008**: A download MUST still be matched to its catalog metadata — poster, media type,
  year, rating, and who sent it — after the naming change.
- **FR-009**: A download's size MUST still be recorded against it when it completes.
- **FR-010**: The Tasks list MUST still show the title of a download. Where a destination now
  ends in a season folder, the title MUST come from the show's folder, NOT from the season
  folder — otherwise every episode reads as "Season 01".
- **FR-011**: Downloads sent before this change MUST keep working: their existing destinations
  are still matched, still attributed, and still titled correctly.
- **FR-012**: Discover's "already have it" marker MUST continue to match, both for folders in
  the old naming and the new.
- **FR-013**: Per-user folder permissions MUST still be enforced on the destination, including
  the deeper season path.

## Success Criteria *(mandatory)*

- **SC-001**: A movie sent from Discover lands in `Title (Year)`, matching what a tidied library
  already looks like.
- **SC-002**: A season sent from Discover lands in `Show (Year)/Season NN`.
- **SC-003**: A tidied library stays tidy: repeated sends never re-introduce the old shape.
- **SC-004**: Every existing download keeps its poster, media type, owner, and title in the
  Tasks list.
- **SC-005**: An episode's row in the Tasks list shows the show's title, never "Season 01".

## Credential-Safety Impact

Required by constitution Principle III.

- **What changes.** Only the SHAPE of a destination folder path that SynoDL already builds and
  already sends to the NAS. No new DSM API, no new outbound host, no new stored field, and no
  change to what is persisted about a download beyond the path string itself.
- **The permission boundary is unchanged.** A destination is still checked against the user's
  folder grants before anything is created or sent, and the deeper season path is checked the
  same way — a season subfolder cannot be used to reach outside a permitted parent.
- **Path construction.** Both the title and the season segment are derived from values SynoDL
  controls — the catalog title and the season parsed from the download's own file names — and
  each is run through the existing single-segment folder-name sanitiser, so neither can
  introduce a separator or traverse upward.
- **What could appear in logs.** Nothing new. Destinations are not logged today and must not
  start being logged.

## Assumptions

- Existing destinations are left alone. This changes only what NEW sends create; the library
  tidy tool (`scripts/library_tidy.py`) is what corrects historical folders.
- Plex tolerates episodes without a season folder, so FR-006's fallback degrades safely.
- The catalog's raw title stays the source of the name, so the "already have it" matcher — which
  normalises punctuation and year form on both sides — keeps matching across the change.

## Out of Scope

- Renaming the downloaded FILES. Only the destination folder is chosen by SynoDL; the file name
  comes from the source.
- Migrating existing destinations, which is the tidy script's job.
- Any change to the movies/TV parent folders themselves, which stay operator configuration.
