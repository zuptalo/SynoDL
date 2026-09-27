# Feature Specification: Download states named plainly, and every failed download retried in one tap

**Feature Branch**: `feat/1049-clearer-download-states`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Instead of saved, failed, downloading, and nothing for when in the queue, use Finished, Failed, Pending and Downloading, if any other fully lower cased UI states there is, make them first letter capital as well please. Let's also add retry failed in the options"

## Overview

A YouTube download's state chip said "saved", "waiting its turn", "starting"
in lower case, next to NAS task chips that say "Finished", "Waiting", "Paused".
The two lists sit on one screen and read as one, so a track and a torrent that
are both done should be called the same thing, spelt the same way. The row chip
and the detail sheet each carried their own copy of the words, so they could
drift — and an unmapped state rendered as an empty chip.

Retrying failed downloads was per row (a swipe) or per playlist (a button
inside it, spec 2035). With several failed playlists, that is still one visit
each; the Tasks menu (⋯) already does "Pause all" / "Clear finished (N)" across
the list, and "Retry failed (N)" belongs beside them.

## Requirements

- **FR-001**: The YouTube download states MUST be shown as **Finished**,
  **Failed**, **Downloading**, **Pending** (queued behind others), **Starting**
  (worker starting) and **Reading contents** (a playlist being enumerated) —
  every label starting with a capital letter, on the row chip and the detail
  sheet alike, from one shared table typed against the state union so no state
  can render empty.
- **FR-002**: A playlist's progress line MUST use the same word: "38 of 340
  finished · 2 failed".
- **FR-003**: The Tasks menu MUST offer **Retry failed (N)**, where N counts
  failed tracks (a failed single is one; a playlist counts its failed tracks),
  and one tap retries every failed single and every playlist's failed tracks
  through the existing retry — each on its own, so one refusal stops nothing
  else. The item is shown with (0) when there is nothing to retry, like
  "Clear finished (0)".
- **FR-004**: The NAS-only selection menu is unchanged (FR-028 of spec 0012:
  NAS bulk actions never reach a YouTube download).

## Out of scope

- Renaming NAS statuses; they were already capitalised.
- Retrying from the selection menu, which is NAS-only by design.
