# Feature Specification: Removed videos are recognised however YouTube words it, and failed tracks lead a playlist

**Feature Branch**: `feat/1051-unavailable-videos-recognised`

**Created**: 2026-09-28

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "for https://www.youtube.com/watch?v=vKNZqM0d-xo it says didn't complete, but when I check youtube it says This video isn't available anymore! also please make sure in a playlist with failed downloads, the failed ones show at the top of the playlist's detailed view, now they are at the end!"

## Overview

One production track sat on "the download did not complete" through eight
retries. Running the worker image against it by hand shows why: yt-dlp
prints `ERROR: [youtube] vKNZqM0d-xo: This video is unavailable`, and the
reason mapping knew only "Video unavailable". YouTube words a removal several
ways — "is unavailable", "isn't available anymore", "is no longer available" —
and the difference between them decided whether a playlist could be cleared
(spec 1050) or kept being retried.

A playlist's sheet listed its tracks in queue order, so with a few hundred
saved tracks the failed ones — the only ones there is anything to do about —
were at the bottom.

## Requirements

- **FR-001**: The failure mapping MUST classify every wording of a removed
  video as "no longer available": "video unavailable", "video is unavailable",
  "isn't available", "is not available", "no longer available", "not available
  anymore", alongside the existing private/removed/terminated forms.
- **FR-002**: A playlist's sheet MUST list failed tracks first, then the ones
  in progress, then those waiting, then the finished — the Tasks list's own
  order with failed brought to the top, since in a playlist they are what the
  reader came to see. Within each band the queue order is kept.
- **FR-003**: A track already recorded as "did not complete" is not rewritten:
  its worker output is gone. Retrying it once after this ships re-fails it with
  the recognised reason, which is what makes its playlist clearable.

## Out of scope

- Reclassifying stored generic failures from the store alone; the evidence
  (worker output) does not survive the Job.
