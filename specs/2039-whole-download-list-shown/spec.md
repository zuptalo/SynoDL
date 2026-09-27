# Feature Specification: The whole download list is shown, not the newest page of it

**Feature Branch**: `fix/2039-whole-download-list-shown`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "are we showing the whole tasks list even when we have hundreds of tasks or maybe is it loaded as scrolled? that should not be the case since it will limit our view to the visible portion of the tasks only"

## Overview

The NAS task list was already whole: the server asks DSM for every task and
the page renders every row. The YouTube list was not. The server pages it
(spec 0013: history is unbounded) at fifty top-level rows, and the client read
the first page and never followed the cursor — so once a user had more than
fifty playlists, the oldest silently vanished from the Tasks list. Not only
from view: the status sort, "Clear finished (N)" and "Retry failed (N)" only
saw what was fetched. Production stood at forty-nine.

Inside a playlist the tracks sheet loaded a hundred and fetched the rest as
the reader scrolled, so a long playlist could not be read whole either.

## Requirements

- **FR-001**: The Tasks list MUST show every YouTube download the user may
  see, however many there are: the client follows the server's cursor to the
  last page and swaps the whole set in at once (no partial repaint).
- **FR-002**: A playlist's sheet MUST show every track, loaded whole before it
  is shown — never as the reader scrolls.
- **FR-003**: Both endpoints MUST honour a `limit`, clamped to a ceiling of
  500 per page; a request ABOVE the ceiling gets the ceiling, not the default
  (the old rule turned "500" into "50").
- **FR-004**: The wire stays paged (spec 0013 FR-006a is unchanged); only the
  client's reading of it changes.

## Out of scope

- Virtualising the rendered list. Hundreds of rows render fine; the request is
  that they all be there.
