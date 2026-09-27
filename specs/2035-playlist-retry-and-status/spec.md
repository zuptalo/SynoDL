# Feature Specification: A playlist's failed tracks are one tap to retry, and the playlist always says what its tracks add up to

**Feature Branch**: `fix/2035-playlist-retry-and-status`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Now there are some failed tracks inside some playlist downloads, check and see why they have failed, also we should be able to retry the failed ones in a more easy way than swipe left and retry on each, also tapping retry on an failed item doesn't dismiss the swiped visual back to normal, and also when the status of a failed item in a downloading playlist is saved after retry, it doesn't update the whole playlist download to saved today! The playlist status should always show the aggregation of everything underneath it"

## Overview

After spec 2034 unblocked the queue, 87 tracks across 13 playlists showed as
failed — every one of them with the same reason, "the download did not
complete", which is all a Job's status can say.

**Why they failed** (re-probed 2026-09-27 with the pinned yt-dlp image, 79
distinct videos): 61 download fine now — YouTube turned the requests away during
the bulk run (HTTP 403/429, bot checks) and a retry is all they need. 18 are
permanent: 12 "Video unavailable", 3 private, 1 Music-Premium-only, 1 paid, 1
age-restricted. The worker printed exactly this on its `ERROR:` line; nothing
kept it.

Four defects, then, and one of them is a feature gap:

1. **The reason is thrown away.** A refusal worth retrying reads the same as a
   video that no longer exists.
2. **Retrying a playlist's failures means one swipe per track.** The server has
   always been able to retry a group's failed items in one call; the UI only
   offered it on a playlist that had finished AND failed, and nowhere in the
   playlist's own sheet.
3. **A tapped swipe action leaves the row slid open**, over a row whose state has
   just changed. The NAS task row already closes itself; this one never did.
4. **A playlist that failed stays failed.** Its state was derived from its items
   only until it first became final — "once final they stay final". Retry
   reopens a track, so final is not forever: a playlist whose last failure had
   been retried and saved still said failed, and was dropped from live updates.

## Requirements

- **FR-001**: A failed download's reason MUST come from what its worker said,
  when it said something recognisable, in plain language: turned away (retry
  later), no longer available, region, signed-in adults only, paying members
  only. Otherwise the generic reason stays. The output is read once, when the
  failure is recorded, before the Job is removed (spec 2034).
- **FR-002**: A playlist's state MUST always be what its tracks add up to:
  downloading while any track is not final (including a retried one), otherwise
  failed if any failed, otherwise saved. A playlist whose stored state disagrees
  with its tracks — including ones already stuck before this fix — MUST be
  corrected by the reconciler, and a playlist that agrees MUST NOT be rewritten
  or re-announced.
- **FR-003**: Reopening a finished playlist MUST clear its ending (reason and
  finish time), so that when it finishes again it says when it really did.
- **FR-004**: A playlist with any failed track MUST offer retrying all of them in
  one tap — from its sheet ("Retry N failed") and from its row's swipe — whether
  or not the rest of it has finished. Only failed tracks are re-queued.
- **FR-005**: Tapping a swipe action on a YouTube row MUST slide the row closed.

## Out of scope

- Retrying automatically. Most of these failures were transient, and a single
  delayed automatic retry for "turned away" is a reasonable next step — but it
  changes how many requests SynoDL sends YouTube, and is its own decision.
- Cookies/sign-in for age-restricted or members-only videos.
