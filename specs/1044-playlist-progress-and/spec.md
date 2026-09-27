# Feature Specification: A playlist shows how much of it is saved, and sorting by status puts what is running on top

**Feature Branch**: `feat/1044-playlist-progress-and`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "I think in case of playlists, it would be nice to show a progressbar on each on in the tasks based on how many tracks has beed saved so far, also look at the sorting functionality there and see why that is not working as expected and fix it so we can more easily sort by the items which do have an active download happening inside them as well"

## Overview

With a few thousand tracks queued across ~50 playlists, the Tasks list could not
answer "which of these is actually downloading?".

**Why sorting did not help** — two causes:

1. **Every unfinished playlist reads "downloading".** A group's state is
   `downloading` from expansion until its last track finishes, including the
   hours it waits behind every other queued track. On production, 50 playlists
   said downloading while at most two had a track running. Nothing told them
   apart: the server's counts had `remaining` (queued + running) but no count
   of what was running now.
2. **The status sort ran backwards for its default order.** It ranked active
   work 0 and finished/failed highest, and the filter sheet's order defaults to
   Descending — so choosing "Status" and nothing else listed finished and failed
   rows first. The NAS list shared the same ranking and the same fault.

## Requirements

- **FR-001**: A playlist's counts MUST include `active` — tracks scheduled or
  downloading right now.
- **FR-002**: A playlist row MUST show a progress bar of saved ÷ total while it
  is not entirely saved (failed included), without a separate percentage — the
  summary already says "N of M saved".
- **FR-003**: A playlist with no track running MUST read "waiting its turn"
  rather than "downloading". An older server without `active` leaves the state
  as it was.
- **FR-004**: Sorting by status in the DEFAULT order (Descending) MUST put active
  work first and finished/failed last, in both the YouTube and the NAS lists;
  Ascending reverses it.
- **FR-005**: In that sort a playlist with a track running MUST rank as
  downloading, and one with none running as waiting.

## Out of scope

- Changing a playlist's stored state: `downloading` still means "not finished";
  what changes is how it is shown and sorted.
