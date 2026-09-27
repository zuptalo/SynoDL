# Feature Specification: Tasks ordered by what is happening, and playlists that say why they failed

**Feature Branch**: `feat/1048-tasks-ordered-what`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "in progress should show at the top, then followed by pendings, and then at the bottom we should see the ones that are finished, and then at the end the ones that have failed after retries and everything. And then we probably should bring up the final status that, okay, this playlist for instance was failed because there were some unavailable videos so the user can see in one go that it's not worth retrying manually again."

## Overview

With a few dozen playlists in the list, the default order — by when each was
added (spec 2031) — put finished and failed playlists above the one actually
downloading, and a failed playlist ended with "some items could not be
downloaded", which said nothing about whether a retry would help.

Spec 1044 already built the ordering (the "Status" sort: downloading, waiting,
saved, failed; a playlist counts as downloading only while a track in it runs)
and spec 2035 gave every failed track a real reason. This makes the first the
default and folds the second into one line on the playlist.

## Requirements

- **FR-001**: The default order of the Tasks list MUST be by what is happening:
  in progress, then waiting, then finished, then failed — newest first within
  each, so whatever was started last still leads its group (refines 2031).
  "Creation date" stays available in the filter sheet.
- **FR-002**: A device holding the old default as a saved choice MUST be
  migrated to the new default once; a sort the user actually picked MUST be
  left alone.
- **FR-003**: A playlist with failed tracks MUST say why, folded from the
  tracks' own reasons: "4 could not be downloaded: 3 no longer available,
  1 adults only" — most common reason first, derived on every read so
  playlists that failed before this say it too, and one still running says
  what has failed so far.
- **FR-004**: A playlist's finished notification MUST carry the same line
  ("45 saved, 1 could not be downloaded: no longer available").

## Out of scope

- Hiding "Retry N failed" when every failure is permanent. The line says so;
  the button stays, since a permanent failure can still change on YouTube's side.
