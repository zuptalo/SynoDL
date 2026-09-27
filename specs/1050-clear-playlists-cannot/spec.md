# Feature Specification: Clear the playlists that can never finish, and retry refused downloads sooner

**Feature Branch**: `feat/1050-clear-playlists-cannot`

**Created**: 2026-09-28

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "do we have a way to cleanup all the hopeless failed playlists now if we wanted to cleanup all of the failed ones which can not reach the final Finished state since some tracks can't be downloaded anymore and everything else which could has already been downloaded? also I think we should reduce the wait time to 30 seconds or so, no need to wait that long for a retry!"

## Overview

After spec 1048 a failed playlist says why: "4 could not be downloaded: 3 no
longer available, 1 adults only". In production, 37 playlists sit in that
state — 73 of their 3266 tracks are gone from YouTube or gated behind age or
payment, everything else is on the NAS, and no retry will ever change that.
Each had to be dismissed with a swipe. "Clear finished" does not touch them,
because they are not finished and never will be.

Separately, a download YouTube turned away (a bot check, a 403/429) was retried
by itself after thirty minutes × attempts (spec 1043). That is far longer than
the refusals last in practice; the operator asked for about thirty seconds.

## Requirements

- **FR-001**: The server MUST mark a playlist **unrecoverable** when it is
  failed, nothing in it is running or waiting, and every remaining failure is
  permanent — the video removed, age-gated, paying-members-only or
  region-locked. A refusal, or a failure with no known cause, is NOT permanent:
  a playlist holding one is never marked, since a retry may well save it.
- **FR-002**: The Tasks menu MUST offer **Clear failed for good (N)**, which
  after a confirmation that says what it does (saved tracks stay on the NAS)
  dismisses every unrecoverable playlist and nothing else.
- **FR-003**: The automatic retry of a refused download MUST default to
  30 seconds × attempts (`YTDL_AUTO_RETRY_AFTER_SECONDS`, was 1800); the
  attempt cap is unchanged. Operators who set the variable keep their value.

## Out of scope

- Hiding or disabling "Retry N failed" on an unrecoverable playlist. The
  reason line says so; clearing is now one tap away.
